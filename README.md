# http2tcp

**http2tcp** is a simple client & server program that turns a HTTP connection to a TCP connection.

This is kind of useful if you want hide all ports traffic other than the standard well-known ports 80 and 443.

## Usage

```shell-session
$ ./http2tcp -h
Usage of http2tcp:
  -c, --client               Run as client.
  -d, --destination string   The destination address to connect to
  -e, --endpoint string      Server endpoint.
  -h, --help                 Show this help
  -l, --listen string        Listen address (client & server)
  -s, --server               Run as server.
  -T, --token string         The token used between client and server
```

Some flags are shared between the client and server.

### Example: Proxy SSH connections

On server:

```shell-session
$ ./http2tcp -s -T $TOKEN -l $SERVER_IP_OR_DOMAIN:2222
```

On client:

```shell-session
$ ./http2tcp -c -d localhost:22 -e $SERVER_IP_OR_DOMAIN:2222 -T $TOKEN
SSH-2.0-OpenSSH_7.6p1 Ubuntu-4ubuntu0.3
```

Now the http2tcp client is connected to the SSH server running on the server side via an HTTP connection.

In your ssh_config, if you write the following:

```ssh_config
Host some-host
	ProxyCommand http2tcp -c -d localhost:22 -e $SERVER_IP_OR_DOMAIN:2222 -T $TOKEN
```

You can now SSH into your server via HTTP.

### Multiple client connections

The client does support multiple connections, just make use of the `-l` flag.
