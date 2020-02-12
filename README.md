What's modified?
================
websocketd will listen on STDIN for commands of this nature:
```
a1.1.1.1
r1.1.1.2
```

`a` adds an IP to the blacklist, `r` removes an IP from the blacklist.

The IP is checked in the Accept() loop so it has a low overhead, although it should be remembered this is still application layer and therefore TCP connection overhead has been suffered by this point.

Why?
----
To allow [Hot Pocket](https://github.com/HotPocketDev/core)  to ban users or peers before tls and websocket handshaking occurs.

