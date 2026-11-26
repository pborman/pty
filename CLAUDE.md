# Notes for working on pty

## Client and server are different builds

pty is one binary, but a session's server runs for weeks while clients come
and go.  A newly built client talking to a server built long ago is the normal
case, not a corner case.  Never assume the other end was built from the same
source as the code you are editing.

## The protocol version

Each side tells the other its protocol version when a connection is made:
the server sends `versionMessage` first, and the client answers with its own.
`version.go` holds `protocolVersion`, the exchange's rules, and the
`peerVersion` type each side keeps for the other end.  A peer that says
nothing is version 0.

The level of a connection is the lower of the two versions.  It belongs to the
connection, not to the session: one server can have several clients attached
at different levels at the same time.

Rules:

*  Anything that needs both ends to cooperate gets a version bump.  That means
   a new message kind, a change to the payload or meaning of an existing kind,
   or a new expectation about what the other end does in response to
   something.
*  A bump comes with a named constant for the feature in `version.go`
   (`versionClear` is the model) set to the new `protocolVersion`.
*  Code that uses such a feature must first check that the connection is at
   that version or beyond, with `Supports(versionX)` on the peer's
   `peerVersion` (the client's `server` in `main.go`, the server's
   `Client.peer`), and must do something sensible when it is not: normally
   skip the feature silently and behave as the older version did.
*  The server checks per client.  When it sends something to several clients,
   each recipient's level is checked separately; one new client does not make
   a feature safe to send to the others.
*  The client must not send a kind the server does not know.  An old server
   answers an unknown kind with an error that is printed on the user's
   terminal.
*  Message kind numbers are part of the wire protocol.  Add new kinds at the
   end with the next number.  Never change, reuse or reorder a number.
*  A change confined to one end (how the client reads the keyboard, how the
   server records the screen) needs no bump.

Test a both-ends feature against a simulated older peer as well as a current
one; `version_test.go` shows how (`TestClientOldServer`,
`TestServerOldClient`).
