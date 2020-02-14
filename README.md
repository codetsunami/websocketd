What's modified?
================

Firewall feature
----------------

websocketd will listen on STDIN for commands of this nature:
```
a1.1.1.1
r1.1.1.2
```

`a` adds an IP to the blacklist, `r` removes an IP from the blacklist.

The IP is checked in the Accept() loop so it has a low overhead, although it should be remembered this is still application layer and therefore TCP connection overhead has been suffered by this point.

Size header feature
-------------------
--sizeheader enforces a 4 byte big endian frame size header on stdin and stdout of the cgi process. 

So the cgi process must send [XX YY ZZ NN] representing the size of the frame it wishes to send, followed by exactly 0xXXYYZZNN bytes. The next frame starts immediately after. And likewise websocat will send frames to the process on the process's stdin in this format.

Why?
----
To allow [Hot Pocket](https://github.com/HotPocketDev/core)  to ban users or peers before tls and websocket handshaking occurs.


