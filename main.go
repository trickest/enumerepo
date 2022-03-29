package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/schollz/progressbar/v3"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

type RateLimit struct {
	Limit     int
	Remaining int
	Cost      int
	ResetAt   time.Time
}

type Repository struct {
	URL             string
	SshUrl          string
	Name            string
	IsEmpty         bool
	PrimaryLanguage struct {
		Name string
	}
	Owner struct {
		URL string
	}
}

type RepoResult struct {
	URL             string `json:"url"`
	SshUrl          string `json:"ssh_url"`
	PrimaryLanguage string `json:"language"`
}

type UserResult struct {
	URL   string        `json:"user"`
	Repos *[]RepoResult `json:"repos"`
}

type RateLimitQuery struct {
	RateLimit RateLimit `graphql:"rateLimit"`
}

type ReposQuery struct {
	RateLimit RateLimit `graphql:"rateLimit"`
	Search    struct {
		RepositoryCount int
		PageInfo        struct {
			EndCursor   githubv4.String
			HasNextPage bool
		}
		Edges []struct {
			Node struct {
				Repo Repository `graphql:"... on Repository"`
			}
		}
	} `graphql:"search(query: $query, type: REPOSITORY, first: 100, after: $after)"`
}

var (
	githubCreateDate = time.Date(2008, 2, 8, 0, 0, 0, 0, time.UTC)
	httpClient       *http.Client
	githubV4Client   *githubv4.Client
	requestDelay     int
	adjustDelay      bool
	rateLimit        *RateLimit
	outputFile       string
	bar              = &progressbar.ProgressBar{}
	barInitialized   = false
	reposToGet       int
	reposRetrieved   int
	firstRound       = true
	delayMutex       = &sync.Mutex{}
	silent           bool
	results          = make([]UserResult, 0)
	resultsMutex     = &sync.Mutex{}
)

func adjustDelayTime(rateLimit RateLimit) {
	remainingRepos := reposToGet - reposRetrieved
	remainingRequests := remainingRepos + remainingRepos/100 + 1
	if remainingRequests < rateLimit.Remaining {
		requestDelay = 0
	} else {
		if rateLimit.Remaining == 0 {
			handleGraphQLAPIError(nil)
		}
		untilNextReset := rateLimit.ResetAt.Sub(time.Now()).Milliseconds()
		if untilNextReset < 0 {
			untilNextReset = time.Hour.Milliseconds()
		}
		requestDelay = int(untilNextReset)/rateLimit.Remaining + 1
	}
}

func doQuery(result *ReposQuery, variables *map[string]interface{}) {
errHandle:
	start := time.Now()
	err := githubV4Client.Query(context.Background(), result, *variables)
	duration := time.Since(start).Milliseconds() - int64(time.Millisecond)
	delayMutex.Lock()
	if err != nil {
		handleGraphQLAPIError(err)
		delayMutex.Unlock()
		goto errHandle
	}

	if firstRound {
		reposToGet += result.Search.RepositoryCount
	} else if adjustDelay {
		adjustDelayTime(result.RateLimit)
	}
	sleep := int64(requestDelay*result.RateLimit.Cost)*int64(time.Millisecond) - duration
	delayMutex.Unlock()
	time.Sleep(time.Duration(sleep))
}

func addRepo(user *UserResult, repo *Repository) {
	*user.Repos = append(*user.Repos, RepoResult{
		URL:             repo.URL,
		SshUrl:          repo.SshUrl,
		PrimaryLanguage: repo.PrimaryLanguage.Name,
	})
}

func addUser(user UserResult) {
	resultsMutex.Lock()
	results = append(results, user)
	resultsMutex.Unlock()
}

func getRepos(query string, startingDate time.Time, endingDate time.Time, userRes *UserResult, wg, barrier *sync.WaitGroup) {
	var reposQuery ReposQuery
	querySplit := strings.Split(query, "created:")
	query = strings.Trim(querySplit[0], " ") + " created:" +
		startingDate.Format(time.RFC3339) + ".." + endingDate.Format(time.RFC3339)
	variables := map[string]interface{}{
		"query": githubv4.String(query),
		"after": (*githubv4.String)(nil),
	}
	doQuery(&reposQuery, &variables)

	maxRepos := reposQuery.Search.RepositoryCount
	if maxRepos == 0 {
		return
	}

	if userRes == nil {
		repos := make([]RepoResult, 0)
		userRes = &UserResult{
			URL:   reposQuery.Search.Edges[0].Node.Repo.Owner.URL,
			Repos: &repos,
		}
		barrier.Done()
		defer wg.Done()
		defer addUser(*userRes)
	}

	barrier.Wait()
	delayMutex.Lock()
	if !barInitialized {
		firstRound = false
		bar = progressbar.NewOptions(reposToGet,
			progressbar.OptionSetDescription("Downloading results..."),
			progressbar.OptionSetItsString("repos"),
			progressbar.OptionShowIts(),
			progressbar.OptionShowCount(),
			progressbar.OptionOnCompletion(func() { fmt.Println() }),
		)
		barInitialized = true
	}
	delayMutex.Unlock()

	if maxRepos >= 1000 {
		dateDif := endingDate.Sub(startingDate) / 2
		getRepos(query, startingDate, startingDate.Add(dateDif), userRes, nil, nil)
		getRepos(query, startingDate.Add(dateDif), endingDate, userRes, nil, nil)
		return
	}

	reposCnt := 0
	for _, nodeStruct := range reposQuery.Search.Edges {
		if nodeStruct.Node.Repo.IsEmpty {
			continue
		}

		addRepo(userRes, &nodeStruct.Node.Repo)

		reposCnt++
		_ = bar.Add(1)
	}

	variables = map[string]interface{}{
		"query": githubv4.String(query),
		"after": githubv4.NewString(reposQuery.Search.PageInfo.EndCursor),
	}
	for reposCnt < maxRepos {
		doQuery(&reposQuery, &variables)

		if len(reposQuery.Search.Edges) == 0 {
			break
		}
		for _, nodeStruct := range reposQuery.Search.Edges {
			if nodeStruct.Node.Repo.IsEmpty {
				continue
			}

			addRepo(userRes, &nodeStruct.Node.Repo)

			reposCnt++
			_ = bar.Add(1)
		}

		variables["after"] = githubv4.NewString(reposQuery.Search.PageInfo.EndCursor)
	}
}

func handleGraphQLAPIError(err error) {
	if err == nil || strings.Contains(err.Error(), "limit exceeded") {
		untilNextReset := rateLimit.ResetAt.Sub(time.Now())
		if untilNextReset < time.Minute {
			rateLimit.ResetAt = time.Now().Add(untilNextReset).Add(time.Hour)
			time.Sleep(untilNextReset + 3*time.Second)
			return
		} else {
			writeOutput(outputFile, silent)
			fmt.Println("\n" + err.Error())
			fmt.Println("Next reset at " + rateLimit.ResetAt.Format(time.RFC1123))
			os.Exit(0)
		}
	}
	writeOutput(outputFile, silent)
	fmt.Println("\n" + err.Error())
	os.Exit(0)
}

func writeOutput(fileName string, silent bool) {
	if len(results) == 0 {
		return
	}
	output, err := os.Create(fileName)
	if err != nil {
		fmt.Println(err)
		fmt.Println("Couldn't create output file")
	}
	defer output.Close()

	data, _ := json.MarshalIndent(results, "", "   ")
	err = ioutil.WriteFile(fileName, data, 0755)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if !silent {
		fmt.Println(string(data))
	}
}

func main() {
	token := flag.String("token-string", "", "Github token")
	tokenFile := flag.String("token-file", "", "File to read Github token from")
	usersFile := flag.String("usernames", "", "File to read usernames from")
	flag.StringVar(&outputFile, "o", "", "Output file name")
	flag.BoolVar(&silent, "silent", false, "Don't print output to stdout")
	flag.IntVar(&requestDelay, "delay", 0, "Time delay after every GraphQL request [ms]")
	flag.BoolVar(&adjustDelay, "adjust-delay", false, "Automatically adjust time delay between requests")
	flag.Parse()

	go func() {
		signalChannel := make(chan os.Signal, 1)
		signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM, syscall.SIGKILL)
		<-signalChannel

		fmt.Println("\nProgram interrupted, exiting...")
		os.Exit(0)
	}()

	if (*token == "" && *tokenFile == "") || outputFile == "" {
		fmt.Println("Token and output file must be specified!")
		os.Exit(1)
	}

	if *usersFile == "" {
		fmt.Println("Usernames must be specified!")
		os.Exit(1)
	}
	githubToken := ""
	if *tokenFile != "" {
		file, err := os.Open(*tokenFile)
		if err != nil {
			fmt.Println("Couldn't open file to read token!")
			os.Exit(1)
		}
		defer file.Close()

		tokenData, err := ioutil.ReadAll(file)
		if err != nil {
			fmt.Println("Couldn't read from token file!")
			os.Exit(1)
		}

		githubToken = string(tokenData)
	} else {
		githubToken = *token
	}

	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	httpClient = oauth2.NewClient(context.Background(), src)
	githubV4Client = githubv4.NewClient(httpClient)

	file, err := os.Open(*usersFile)
	if err != nil {
		fmt.Println("Couldn't open file to read users!")
		os.Exit(1)
	}
	defer file.Close()

	userNames := make([]string, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		userName := strings.TrimSpace(scanner.Text())
		userNames = append(userNames, userName)
	}

	if adjustDelay {
		var RLQuery RateLimitQuery
		err = githubV4Client.Query(context.Background(), &RLQuery, nil)
		if err != nil {
			fmt.Println(err)
			fmt.Println("Couldn't get initial rate limit!")
			os.Exit(1)
		}
		rateLimit = &RLQuery.RateLimit
		if len(userNames) < rateLimit.Remaining {
			requestDelay = 0
		} else {
			if rateLimit.Remaining == 0 {
				handleGraphQLAPIError(nil)
			}
			untilNextReset := rateLimit.ResetAt.Sub(time.Now()).Milliseconds()
			if untilNextReset < 0 {
				untilNextReset = time.Hour.Milliseconds()
			}
			requestDelay = int(untilNextReset)/rateLimit.Remaining + 1
		}
	}

	var wg sync.WaitGroup
	var barrier sync.WaitGroup
	for _, userName := range userNames {
		wg.Add(1)
		barrier.Add(1)
		go getRepos("user:"+userName, githubCreateDate, time.Now().UTC(), nil, &wg, &barrier)
	}

	wg.Wait()
	writeOutput(outputFile, silent)
}
