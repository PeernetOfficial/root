# Peernet Root Peer

The root peer client is a fork of the command line client. It adds statistics functionality.

## Compile

To build:

```
go build
```

To cross compile from Windows to Linux and deploy:

```
set GOARCH=amd64
set GOOS=linux
go build

chmod +x ./root
nohup ./root &

ps -ef | grep -i ./root
kill [pid]
```
