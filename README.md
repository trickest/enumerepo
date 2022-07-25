<h1 align="center">enumerepo <a href="https://twitter.com/intent/tweet?text=enumerepo%20-%20Make%20URL%20path%20combinations%20using%20a%20wordlist%20https%3A%2F%2Fgithub.com%2Ftrickest%2Fenumerepo&hashtags=bugbounty,bugbountytips,infosec"><img src="https://img.shields.io/badge/Tweet--lightgrey?logo=twitter&style=social" alt="Tweet" height="20"/></a></h1>
<h3 align="center">List all public repositories for (valid) GitHub usernames</h3>

![enumerepo](enumerepo.png "enumerepo")

Read a list of GitHub usernames and/or organizations, verify their existence, and list the repositories owned by each one. 

# Installation
## Binary
Binaries are available in the [latest release](https://github.com/trickest/enumerepo/releases/latest).

## Docker
```
docker run quay.io/trickest/enumerepo
```

## From source
```
go install github.com/trickest/enumerepo@latest
```

# Usage
```
  -adjust-delay
    	Automatically adjust time delay between requests
  -delay int
    	Time delay after every GraphQL request [ms]
  -o string
    	Output file name
  -silent
    	Don't print output to stdout
  -token-file string
    	File to read Github token from
  -token-string string
    	Github token
  -usernames string
    	File to read usernames from
```

### Example
##### wordlist.txt
```
dev
prod/
admin.py
app/login.html
```

```shell script
$ enumerepo -d example.com -l 2 -w wordlist.txt
example.com/dev
example.com/prod
example.com/dev/dev
example.com/prod/dev
example.com/dev/prod
example.com/prod/prod
example.com/dev/admin.py
example.com/dev/app/login.html
example.com/prod/admin.py
example.com/prod/app/login.html
example.com/dev/dev/admin.py
example.com/dev/dev/app/login.html
example.com/prod/dev/admin.py
example.com/prod/dev/app/login.html
example.com/dev/prod/admin.py
example.com/dev/prod/app/login.html
example.com/prod/prod/admin.py
example.com/prod/prod/app/login.html

```

# Report Bugs / Feedback
We look forward to any feedback you want to share with us or if you're stuck with a problem you can contact us at [support@trickest.com](mailto:support@trickest.com). You can also create an [Issue](https://github.com/trickest/enumerepo/issues/new) or pull request on the Github repository.

# Where does this fit in your methodology?
Enumerepo is an integral part of the [Insiders](https://github.com/trickest/insiders) workflow many workflows in the Trickest store. Sign up on [trickest.com](https://trickest.com) to get access to these workflows or build your own from scratch!

[<img src="./banner.png" />](https://trickest-access.paperform.co/)
