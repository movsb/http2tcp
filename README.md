# http2tcp

**http2tcp** is a simple client & server program that turns an HTTP connection to a TCP connection.

This is kind of useful if you want to hide all ports traffic other than the standard well-known ports 80 and 443.

## Usage

```shell-session
$ ./http2tcp
Usage of http2tcp:
  -s, --server               Run as server. [S]
  -c, --client               Run as client. [C]
  -e, --endpoint string      Server endpoint. [C]
  -k, --key string           The Server PrivateKey [S] or PublicKey [C].
      --user-agent string    Use this User-Agent instead of the default Go-http-client/1.1 [C]
  -l, --listen string        Listen address [SC]
  -d, --destination string   The destination address to connect to [C]
      --keygen               Generate a new private key pair
  -h, --help                 Show this help

[S]: server side flag.
[C]: client side flag.
```

Some flags are shared between the client and server.

## Example: Proxy SSH connections

### On Server

Automatically generate a pair of PublicKey and PrivateKey:

```shell-session
$ ./http2tcp -s -l SERVER_IP_OR_DOMAIN:2222 
2022/11/26 14:52:32 main.go:59: Public  Key: XOK89m7YytIM2NjfhqAe4FoUHVWabjmOF3eVpnFnb28
2022/11/26 14:52:32 main.go:60: Private Key: gKOg8zeuuB73X9FBgMb-xAUGxnv8x7y6WJ3_FI_0Msc
```

Or, specify a PrivateKey to use:

```shell-session
$ ./http2tcp -s -l SERVER_IP_OR_DOMAIN:2222 -k gKOg8zeuuB73X9FBgMb-xAUGxnv8x7y6WJ3_FI_0Msc
2022/11/26 14:56:49 main.go:57: Public  Key: XOK89m7YytIM2NjfhqAe4FoUHVWabjmOF3eVpnFnb28
```

### On Client

Specify server PublicKey to use with `-k` option:

```shell-session
$ ./http2tcp -c -d localhost:22 -e $SERVER_IP_OR_DOMAIN:2222 -k XOK89m7YytIM2NjfhqAe4FoUHVWabjmOF3eVpnFnb28
SSH-2.0-OpenSSH_7.6p1 Ubuntu-4ubuntu0.3
```

The client PublicKey/PrivateKey pair is always generated randomly.

Now the http2tcp client is connected to the SSH server running on the server side via an HTTP connection.

In your ssh_config, if you write the following:

```ssh_config
Host some-host
	ProxyCommand http2tcp -c -d localhost:22 -e $SERVER_IP_OR_DOMAIN:2222 -k XOK89m7YytIM2NjfhqAe4FoUHVWabjmOF3eVpnFnb28
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
ProxyCommand http2tcp -c -d localhost:22 -e https://example.com/some-path/ -k xxxxxxxx
```

## Security

* The Destination is in the HTTP POST body, not in the GET URL parameter.
* The Destination is asymmetrically encrypted, and is only known by the client & the server, not NGINX.
* The connection data of Destination is **not** encrypted currently for performance considerations.
* Supporting for encrypting the connection data is planned.
