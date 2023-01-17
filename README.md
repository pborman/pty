# pty

I wrote this mainly for personal use for working on linux machines from my macOS desktop.  Github is a convenient way to import it on my corporate appliance.  As this is a personal project it has some areas that might be a little messy and is very short on godoc comments.  pty is only supported on Linux as it utilizes /proc.

Typical invocation: ```ssh -t remotehost path-to-pty```

pty is a ```screen``` like program for managing sessions on a remote machine.  It uses ```<ctrl-p>``` as the escape character. ```<ctrl-p>.``` is used to disconnect.  Use ```<ctrl-p>:``` to execute a pty command.  The commands are:
```
  dump   - dump stack
  excl   - detach all other clients
  env    - display environment variables
  list   - list all clients
  save   - save buffer to FILE
  setenv - forward environment variables
  ssh    - forward SSH_AUTH_SOCK
  tee    - tee all future output to FILE (- to close)
  ps     - display processes on this pty
```
pty is both a client and server.  The first time pty is called (or anytime when there are no sessions) it will ask for a session:
```
Name of session to create (or shell): 
```
Specifying shell just execs your login shell (you should have just used ssh without pty).  The session name must not contain slashes.  Once a session name is chosen, pty will fork and fork again itself as a pty server.  The server spawns an interactive shell.  The original pty (the client) then connects to the server forwards standard in/out/error between the login shell and the client.  The connection is over TCP on the loopback interface (127.0.0.1).  The address of the server is written to ```$HOME/.pty/session-SESSION-NAME```.  If there are sessions existing, pty asks you to select a session:
```
Current sessions:
   -1) Spawn /usr/bin/ksh
    0) Create a new session
    1) debugging (1 Client)
         pty 368978 (~)
          ⬑ [-ksh] (~)
            ⬑ vi [interesting.go] (~)
Please select a session: 
```
This shows there is only one pty session available and it is editing the file interesting.go.  It is possible for multiple clients to be attached to a single pty session, though visual editing can become interesting.

pty keeps its log files in ```$HOME/.pty/log```.
