# http2tcp

**http2tcp** is a simple client & server program that turns an HTTP connection to a TCP connection.

This is kind of useful if you want to hide all ports traffic other than the standard well-known ports 80 and 443.

## Usage

```shell-session
$ ./http2tcp -h
Usage of http2tcp:
  -s, --server               Run as server.
  -c, --client               Run as client.
  -l, --listen string        Listen address (client & server)
  -e, --endpoint string      Server endpoint.
  -d, --destination string   The destination address to connect to
  -t, --token string         The token used between client and server
  -h, --help                 Show this help
```

Some flags are shared between the client and server.

### Example: Proxy SSH connections

On server:

```shell-session
$ ./http2tcp -s -t $TOKEN -l $SERVER_IP_OR_DOMAIN:2222
```

On client:

```shell-session
$ ./http2tcp -c -d localhost:22 -e $SERVER_IP_OR_DOMAIN:2222 -t $TOKEN
SSH-2.0-OpenSSH_7.6p1 Ubuntu-4ubuntu0.3
```

Now the http2tcp client is connected to the SSH server running on the server side via an HTTP connection.

In your ssh_config, if you write the following:

```ssh_config
Host some-host
	ProxyCommand http2tcp -c -d localhost:22 -e $SERVER_IP_OR_DOMAIN:2222 -t $TOKEN
```

You can now SSH into your server via HTTP.

### Multiple client connections

The client does support multiple connections, just make use of the `-l` flag.

### Behind NGINX reverse proxy

This is actually the standard way to use HTTP2TCP.

```nginx
server {
	server_name example.com;
	listen 443 ssl http2;
	location = /some-path/ {
		proxy_http_version 1.1;
		proxy_set_header Host $host;
		proxy_set_header Upgrade $http_upgrade;
		proxy_set_header Connection "Upgrade";
		proxy_read_timeout 600s;
		proxy_pass http://localhost:2222/;
	}
}
```

Now the `ProxyCommand` should be:

```ssh_config
ProxyCommand http2tcp -c -d localhost:22 -e https://example.com/some-path/ -t $TOKEN
```
