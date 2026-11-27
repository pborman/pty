# pty tasks

Each task has one status: NEW, STARTED, CODED, TESTED or DONE.  DONE covers
every way a task stops being live; the text says which.  Open tasks come
first, most worth doing first.

Read `CLAUDE.md` before changing anything the client and server both take
part in.

## NEW - Closes that nobody waits for

`checkClose` (`client.go`) closes in a background goroutine and always returns
nil.  Two things go wrong because of it:

*  `Shell.Exit` (`shell.go`) queues each client's last output (the sequence
   that leaves the alternate screen), starts the closes, and exits the process
   at once.  The last output races the exit.  If the shell dies while a
   full-screen program is up, a terminal can be left on the alternate screen.
*  `teeer.Open` (`main.go`) tests the error from `checkClose`, which is never
   set, and a file that is reopened at once may still be open.

What to do: give callers that need it a close that waits and returns the
error.  Have `Exit` wait for the clients' last flush before exiting, for at
most `clientCloseTimeout` (`close_timeout` in config.yaml).  `Client.Close`
already waits that long for one client; `Exit` needs to wait for all of them,
together, not one after another.

## NEW - A shell that fails to start is not reported to the client

Not reproduced; this is from reading the code.  The server publishes its
address (`Session.Listen`) before it starts the shell (`Session.shell` in
`server.go`).  The client now connects as soon as the address appears.  If
`shell.Start` then fails the server exits, and the client sees its connection
close and exits with status 0, having printed only "Connected to session".
Before the client stopped sleeping half a second on start, it usually got a
dial error here instead.

What to do: either start the shell before publishing the address, or have the
server send the error to connected clients in a `serverMessage` before it
exits.  The client should exit non-zero.  Check first that it happens: make
`Start` fail (a shell that cannot be run) and look at what the client prints.

## NEW - Replies are written from the goroutine that reads the connection

In `Shell.attach` (`server.go`) the reader's callback answers with `mw.Send`
and `mw.Sendf`: the count, the ping ack, the ps listing, error text, the
escape list.  These are blocking writes to the socket on the goroutine that
reads it, so a client that stops reading also stops being read.  Nothing holds
the shell lock while it happens, so other clients are not affected.  The
client has the same shape in `clientCommand` (`main.go`).

What to do: on the server, send these through the `Client` queue, as
`Shell.List` does, so the reader never blocks on a write.  Keep the version
message first on the wire (`MessengerWriter.SendFirst`), and note that a
`Session.Command` connection never attaches but must still get its answer.

## NEW - Dead protocol code

*  `waitMessage` is never sent.  Its number (5) stays reserved, but the case
   in `clientCommand` (`main.go`) can go.
*  The `startMessage` case in `Session.Command` (`session.go`) cannot happen:
   `startMessage` is only sent to a client that attached, and `Command` never
   attaches.  It resends the request, which would be wrong if it did happen.

## NEW - parse.Line accepts an unterminated quote

`parse.Line("echo \"unterminated")` returns `["echo" "unterminated"]` and a
nil error.  It should return an error, which the command prompt (`main.go`)
already prints.

## NEW - Show a session's protocol version in --list

Optional.  A server tells every connection its version, including the ones
`--list` makes (`Session.Command`), where it is ignored.  Showing it would let
the user see which sessions predate a feature and need restarting to get it.
Version 0 means the server is older than the version exchange.

## DONE - Give econn an integrity check

Withdrawn.  `econn` encrypts but does not authenticate the stream.  It is only
used on the loopback TCP fallback, where nothing sits between the two ends to
change the bytes, and the handshake already proves both ends hold the secret.

## DONE - Send clears to servers built 2026-11-25 to 2026-11-26

Withdrawn.  Servers built in those two days handle `clearMessage` but, like
all older servers, report version 0, so the client no longer sends it to
them.  They cannot be told apart from older servers, and restarting the
session fixes it.
