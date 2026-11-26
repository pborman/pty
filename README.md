# pty

pty keeps shell sessions running on a machine after you disconnect from it,
and lets you get back to them later, from the same terminal or a different one.
It is in the same family as `screen` and `tmux`, but it does much less.  There
are no windows, no panes and no status line.  You get your shell, and when you
come back you get it again, with the screen the way you left it.

I wrote it for my own use, originally for working on Linux machines from my
Mac, and these days on the Mac itself.  It is a personal project and parts of
it show that, but I use it every day.  It runs on Linux and macOS.

## Installing

You need Go 1.21 or later:

    go install github.com/pborman/pty@latest

The binary is both the client and the server, so there is nothing else to
install or start.  The usual way to use it is straight from ssh:

    ssh -t remotehost pty

The `-t` matters; pty needs a terminal.

## Sessions

The first time you run pty there are no sessions, so it asks for a name:

    Name of session to create (or shell) [Session-1]:

Press return to take the suggested name, or type your own.  A name can use
letters, digits, spaces and `_ - . + ! = : [ ] < > { }`.  It cannot start with
a dot and cannot be `log`.  Typing `shell` (or `sh`) just runs your login shell
without pty, which is handy when you ssh in for something quick.

pty then starts a server for the session in the background.  The server starts
your login shell on a new pseudo-terminal, and your pty connects to it.  When
you disconnect, the server and the shell keep running.  A session ends when its
shell exits.

When there are sessions already, pty lists them and asks which one you want:

    shell) Spawn /bin/ksh
     name) Create a new session named name
        1) build (0 Clients)  (120x40)
             pty 41233 (~/src/pty)
              ⬑ [-ksh] (~/src/pty)
                ⬑ vi [main.go] (~/src/pty)
        2) mail (1 Client) inbox (80x24)
             pty 40112 (~)
              ⬑ [-ksh] (~)
    Please select a session [Session-1]: [1]:

Each session shows how many terminals are attached to it, its title if it has
one, and the size of the terminal that last used it.  Under that is what is
running in it, so you can tell your sessions apart.  Editors show the files
they have open.  Pick a session by number or by name.  A new name creates a
session (pty asks first, unless it is the suggested name).

The numbers in brackets after the prompt are sessions that nobody is attached
to and whose last terminal was the same size as yours.  Press return and you
get the first of them.  If there are none, return creates the suggested new
session.

You can skip the menu:

    pty build          attach to the session named build
    pty -c build       the same, but create it if it does not exist
    pty --new build    create build; it is an error if it already exists

Attaching by name to a session that someone else is attached to asks
`Continue? [y/n]` first.  `-y` answers yes for you.

`--detach` creates a session without connecting to it, for when you want a
session ready for later:

    pty -c --detach build

### Getting back to the right session

If your terminal gives each window or tab an ID, you can pass it with `--id`.
macOS Terminal and iTerm2 set `TERM_SESSION_ID`, so:

    ssh -t remotehost "pty --id $TERM_SESSION_ID"

The double quotes make the ID expand on your Mac, not on the remote machine.
pty remembers the ID of the terminal that last attached to each session.  In
the menu, that session is marked with `+` and becomes the default.  With
`--auto` pty does not show the menu at all.  It attaches to the session with
your ID if nobody else is using it, or else to an unused session whose last
terminal was the same size as yours.  It only shows the menu if neither
exists.

### Listing and cleaning up

    pty --list         the running sessions, with client counts and titles
    pty --mlist        the same as COUNT:NAME lines, for scripts
    pty --dead         sessions whose server is gone (the machine rebooted, say)
    pty --clean        remove the dead sessions

## Inside a session

pty passes everything through to your shell except its escape character,
which is `^P`.

    ^P .     disconnect, leaving the session running
    ^P :     run a pty command
    ^P ^P    send a single ^P to the shell

Use `-e` to pick another escape character, such as `-e '^]'`, `-e x` or
`-e '\035'`.  `-e ''` turns it off entirely.

`^P :` prompts with `Command:`.  The command line is split into words the way
a shell would split it, so quotes work.  The commands are:

    help              list the commands
    title [TEXT]      show the session's title, or set it to TEXT
    list              list the terminals attached to the session (* is you)
    excl              disconnect every other terminal from the session
    ps                show what is running in the session
    env [NAME...]     show your environment (the client's, not the shell's)
    setenv NAME...    type NAME=value into the shell for each NAME, using
                      your environment's value
    ssh               type SSH_AUTH_SOCK=value into the shell
    tee FILE          copy all further output to FILE; "tee -" stops
    save FILE         write what pty has recorded of the screen to FILE
    escapes alt|normal  list the escape sequences in that recorded screen
    dump              write a goroutine dump to the logs

`setenv` and `ssh` really do type the text, followed by return.  Use them at
a shell prompt.  `tee` and `env` run on your side of the connection, and
`save` runs in the server, so give `save` a full path.

Inside a session the shell has `_PTY_NAME` (the session name), `_PTY_SOCKET`
and `_PTY_SHELL` set.  pty will not create or attach to a session from inside
one of its own, but `--list` and the other listing options still work there.

## Coming back to a session

The server records what the shell writes, up to a megabyte each for the
normal screen and for the alternate screen that full-screen programs like vi
use.  When you attach, pty clears your screen and replays the record, so you
see what you would have seen if you had never left.  If a full-screen program
is running, you get its screen too.

The record is reset when the screen is: clearing the scrollback (`ESC [ 3 J`,
which `clear` sends on most systems) empties the normal screen's record, and
erasing the alternate screen empties its record.  If your terminal can tell
pty that it has cleared itself (iPadSSH does this on Cmd-K), pty starts the
record over from what the terminal says it is showing.  The terminal does this
by sending, as input, `ESC ] 7720 ; clear ; BASE64 ST`, where BASE64 encodes
what is now on its screen, ending with the cursor position.

Some things are not recorded, because replaying them later would do harm:

* Questions programs ask the terminal: device attributes, cursor position,
  modes, colors, the window size and title, the terminal's version, and so
  on.  The terminal answers by typing the answer, so replaying a question
  makes your terminal type an old answer into whatever is running when you
  attach.  If a sequence sets one color and asks about another, only the
  setting is kept.
* Window changes: moving, resizing, iconifying.
* The clipboard (OSC 52).  Replaying it would put old text on your clipboard.

All of this applies only to the record.  Everything is passed straight
through to the terminals that are attached when it happens.

If a program dies in the middle of an escape sequence, the record could miss
everything after it.  Typing `^C` ends any sequence pty is still waiting on.

## More than one terminal

Any number of terminals can attach to one session.  They all see the same
output, and whatever any of them types goes to the shell.  The terminal that
typed most recently (or resized its window most recently) is the primary one.
The session's window size follows the primary terminal, so a small window and
a large one will fight over full-screen programs.  `^P : excl` is the cure.

When a program asks the terminal a question, every attached terminal answers.
pty passes on the answers from the primary terminal only, or from the first
terminal to answer if there is no primary one yet.  An answer does not make a
terminal primary, since nobody typed it.

## ssh-agent and other sockets

When you come back to a session from a new ssh login, the shell still has the
`SSH_AUTH_SOCK` of the login that started it, which is gone.  There are two
ways around this.

The better one is to let pty forward the socket.  Put this in
`~/.pty/config.yaml`:

    forward:
      - SSH_AUTH_SOCK

Then `SSH_AUTH_SOCK` in the session's shell points at a socket of pty's own,
and pty passes each connection to it on to the primary terminal's ssh-agent.
That is the agent of whichever terminal you are typing in.  This works for any
variable that names a unix domain socket.  It only happens if the login that
created the session had the variable set; pty does not invent one.

The other way is `^P : ssh` at a shell prompt, which types the current
`SSH_AUTH_SOCK=...` into the shell for you.  A program in the session can ask
for the same thing by writing `ESC [ z`.

## Configuration

pty reads `~/.pty/config.yaml` when it starts.  Besides `forward` above, it
can have a `sequences` list.  Each entry names more sequences to keep out of
the record, and optionally the answer the terminal gives to each, which is
then passed on from one terminal only.  You only need this for something pty
does not already know about, usually a private sequence of some terminal.

    sequences:
      # An OSC: no end needed
      - start: '\e]1337;ReportSomething'
        answer:
          start: '\e]1337;Something='
      # A sequence of its own, with its own ends
      - start: '\e_Gi=1,a=q'
        end: ['\e\\', '^G']
        limit: 65536
        answer:
          start: '\e_Gi=1;'
          end: ['\e\\']

If `start` begins with `ESC [`, `ESC ]` or `ESC P`, the entry covers every
CSI, OSC or DCS that starts with it.  Those end where the terminal ends them,
so they take no `end` or `limit`.  Anything else is a sequence of its own.  It
runs to the first of its ends, or is just `start` if it has no `end`.  If
none of its ends turns up within `limit` bytes (4096 unless you say), pty
decides it was not that sequence after all and records it.  An `answer` is
written the same way.  pty already knows the one sequence of this kind I
needed, iPadSSH's `ropen` query and its `#ROPEN ... NEPOR#` answer.

Write these strings in single quotes, so YAML leaves the backslashes alone,
and pty decodes them itself:

    \e                ESC
    \a \b \f \n \r \t \v \\ \' \"
    \NNN              the byte with octal value NNN
    \xNN              the byte with hex value NN
    \uNNNN            the character U+NNNN, as UTF-8
    ^X                a control character: ^@ through ^_ (or ^a through ^z),
                      and ^? for DEL
    \^                a ^

(YAML's own double-quoted strings would turn `"\004"` into a NUL followed by
`04`, and `"\x9b"` into a two-byte character, which is why pty does it.)  A
mistake in a sequence, such as an `end` on an OSC or a start that overlaps
another one, stops pty at startup with a message saying which entry is wrong.

If `~/.pty/motd` exists, pty shows it before connecting you and waits for you
to press return.

## Files

Everything lives in `~/.pty`, which pty creates and insists is mode 0700.
Each session has a directory, `~/.pty/@NAME`, holding its socket, its pid,
title, last terminal size and ID, and its logs.  The client and server write
logs under `~/.pty/@NAME/log`, and anything logged before a session is chosen
goes in `~/.pty/log`.  Logs are rotated hourly and old ones are removed after
six hours.

Clients talk to the server over a unix domain socket in the session's
directory.  If a socket cannot be made there (the path is too long, or the
home directory is on a filesystem that cannot hold sockets), the server falls
back to TCP on 127.0.0.1.  The connection is then encrypted with a secret kept
in the session's directory, so only someone who can read that directory can
connect.

## When something goes wrong

* `-v` turns on verbose logging.
* `--debug` keeps the server in the foreground instead of putting it in the
  background, and `--debug_server` turns on the server's own debug output.
* `--hexdump=FILE` writes a hex dump of everything passing through the
  client: what you type, what is sent on to the shell, and what comes back.
* `^P : dump` writes a dump of every goroutine to the logs, which is how I
  find hangs.
* `--grok` sends `^H` instead of DEL for the backspace key.  I needed this
  for a program that hangs on DEL when it is not in the Mac's GUI login
  session.  It may help with anything else that is picky about backspace.

## License

Apache 2.0.  See LICENSE.
