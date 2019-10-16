# EZPTT
[![Go Report Card](https://goreportcard.com/badge/github.com/DoubleChuang/EZPTT)](https://goreportcard.com/report/github.com/DoubleChuang/EZPTT)
[![Build Status](https://travis-ci.org/DoubleChuang/EZPTT.svg?branch=master)](https://travis-ci.org/DoubleChuang/EZPTT)

EZPTT is a tool that lets you quickly log in to PTT with just one command line.
### How to use
```console=
git clone https://github.com/DoubleChuang/EZPTT.git
cd EZPTT
make
cd bin/{YourComputingPlatform}

# Enter your account password into PttConfig.csv
echo "YourPttUserId,YourPttUserPassword" > PttConfig.csv
./EZPTT
```

Documentation
====
[EZPTT docs](https://godoc.org/github.com/DoubleChuang/EZPTT/pttclient)


