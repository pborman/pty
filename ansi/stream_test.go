package ansi

import (
	"sort"
	"strings"
	"testing"
)

func TestDecodeStream(t *testing.T) {
	want := input + input2
	d := New(strings.NewReader(want))
	seen := map[string]bool{}
	var read []string
	for {
		s, err := d.Next()
		if err != nil {
			break
		}
		r := Table[s.Code]
		if r == nil {
			r = &Sequence{Name: "?"}
		}
		read = append(read, string(s.Text))
		t.Logf("Text[%s] %q", r.Name, s.Text)
		seen[string(s.Code)] = true
	}
	got := strings.Join(read, "")
	if got != want {
		t.Errorf("Got wrong text, got:\n%q\nwant:\n%q", got, want)
	}
	codes := make([]string, 0, len(seen))
	for code := range seen {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		t.Logf("Code: %q", code)
	}
}

const input = `]2;/Users/prb/src/github.com/pborman/ptyPTY => PTY => vi
./en_US.UTF-8: No such file or directory
[1;40r(B[m[4l[?7h[?1h=]2;/Users/prb/src/github.com/pborman/ptyPTY => `
const input2 = `
MN Linux => 
MN Linux => vi pty   main.go
[?1049h[1;57r(B[m[4l[?7h[?1h=[H[2Jpackage main[3dimport ([4d"bytes"[5;9H"fmt"[6;9H"io"[7;9H"net"[8;9H"os"[9;9H"os/signal"[10;9H"path/filepath"[11;9H"strings"[12;9H"sync"[13;9H"syscall"[14;9H"time"[16;9H"github.com/pborman/pty/parse"[17;9Httyname "github.com/pborman/pty/tty"[18;9H"github.com/kr/pty"[19;9H"github.com/pborman/getopt"[20;9H"golang.org/x/crypto/ssh/terminal"[21d)[23dvar ([24;9Hostate *terminal.State[25;9Htilde  = byte('P' & 0x1f)[26d)[28dfunc main() {[29;9Hdir := filepath.Join(user.HomeDir, rcdir)[30;9Hos.Mkdir(dir, 0700)[31;9Hos.Chmod(dir, 0700)[32;9Hfi, err := os.Stat(dir)[33;9Hif err != nil {[34;17Hexitf("no pty dir: %v", err)[35;9H}[36dif fi.Mode()&0777 != 0700 {[37;17Hexitf("pty dir has mode %v, want %v", fi.Mode(), os.FileMode(os.Mode[38;1HDir|0700))[39d}[41dinternal := getopt.StringLong("internal", 0, "", "internal only flag")[42;9HinternalDebug := getopt.StringLong("internal_debug", 0, "", "internal only f[43;1Hlag")[45;9Hechar := getopt.StringLong("escape", 'e', "^P", "escape character")[46;9HnewSession := getopt.StringLong("new", 0, "", "create new session named NAME[47;1H", "NAME")[48ddebugFlag := getopt.BoolLong("debug", 0, "debug mode, leave server in foregr[49;1Hound")[50d  debugServer := getopt.BoolLong("debug_server", 0, "enable server debugging")[51;9Hdetach := getopt.BoolLong("detach", 0, "create and detach new shell, do not[52dconnect")[53dlist := getopt.BoolLong("list", 0, "just list existing sessions")[54;9Hgetopt.Parse()[56;9Hif *list {[H[57dmain.go: unmodified: line 1[H[26;42r[26;1H[7T[1;57r[57;1H[J[1;32H[1K fmt.Fprintf(w, "%s=%s\r", name, quoteShell(value))[2;25H}[3;16H[1K }[4;9Hcase "ssh":[5;16H[1K if !raw {[6d[1K return[7;16H[1K }[8d[1K if value, ok := os.LookupEnv("SSH_AUTH_SOCK"); ok {[9;24H[1K fmt.Fprintf(w, "SSH_AUTH_SOCK=%s\r", quoteShell(value))[10;16H[1K }[K[11;9Hcase "save":[12;16H[1K if !raw && len(args) != 2 {[13;24H[1K fmt.Printf("usage: save FILENAME\n")[14;24H[1K return[15;17H}[16d[1K if raw && len(args) == 2 {[17;24H[1K w.Send(saveMessage, []byte(args[1]))[18;16H[1K }[K[19;9Hcase "tee":[K[20;16H[1K if raw {[K[21d[1K return[22;17H}[23d[1K if len(args) != 2 {[24;24H[1K fmt.Printf("usage: tee FILENAME\n")[25;24H[1K return[K[26;17H}[27dtee.Open(args[1])[28;9Hdefault:[29dif !raw {[30dfmt.Printf("unknown command: %s\n", args[0])[31;17H}[32;9H}[33d}[35;6HwatchSigwinch(w *MessengerWriter) error {[36;9Hrows, cols, err := pty.Getsize(os.Stdin)[K[37;9Hif err == nil {[K[38;16H[1K w.Send(ttysizeMessage, encodeSize(rows, cols))[39;9H}[K[41;17Hreturn nil[K[43;8H[1K go func() {[44;17Hch := make(chan os.Signal, 2)[45;16H[1K signal.Notify(ch, syscall.SIGWINCH)[K[46;16H[1K for range ch {[K[47;24H[1K rows, cols, err := pty.Getsize(os.Stdin)[48;24H[1K if err != nil {[K[49;32H[1K fmt.Fprintf(os.Stderr, "getsize: %v\r\n", err)[50;24H[1K } else {[K[51d[1K w.Send(ttysizeMessage, encodeSize(rows, cols))[K[52;24H[1K }[53;16H[1K }[K[54;9H}()[K[55;9Hreturn nil[56d}[K[57d?AlreSearching...(B[0;7mPattern not found[13G(B[m[6G[A[57d?[KOpen[J[27;21H[57dSearching...[J[27;21H[1;17Hcase '"', '\\', '$':[K[2;25Hfmt.Fprintf(&buf, "%c", c)[3;17Hdefault:[4d[1K fmt.Fprintf(&buf, "%c", c)[5;17H}[K[6;9H}[K[7dfmt.Fprintf(&buf, "some sort of text")[8;9Hreturn buf.String()[K[9d}[K[10d[K[11dtype teeer struct {[K[12;9Hmu   sync.Mutex[K[13;9Hw    *os.File[K[14;9Hpath string[K[15d}[K[16d[K[17dvar tee teeer[K[18d[K[19dfunc (t *teeer) Write(buf []byte) (int, error) {[20;9Ht.mu.Lock()[K[21;9Hw := t.w[K[22;9Ht.mu.Unlock()[23;9Hif w == nil {[K[24;17Hreturn len(buf), nil[K[25;9H}[K[26dreturn w.Write(buf)[27d}[K[28d[K[29dfunc (t *teeer) Open(path string) {[30;9Hif path == "-" {[K[31;17Ht.mu.Lock()[32;16H[1K if t.w != nil {[33;24H[1K if err := t.w.Close(); err != nil {[34;33Hfmt.Printf("ERROR CLOSING TEE: %v\r\n", err)[35;24H[1K }[K[36;16H[1K }[K[37d[1K t.path = ""[38;17Ht.mu.Unlock()[K[39;16H[1K return[40;9H}[K[41dt.mu.Lock()[K[42;9Hw := t.w[43;9Ht.mu.Unlock()[44;9Hif w != nil {[K[45;17Hfmt.Printf("ERROR: already teeing to %s\r\n", t.path)[46;17Hreturn[K[47;9H}[K[48dw, err := os.Create(path)[K[49;9Hif err != nil {[K[50;17Hfmt.Printf("ERROR OPENING TEE: %v\r\n", err)[51;17Hreturn[K[52;9H}[K[53dt.mu.Lock()[54;9Hif t.w == nil {[55;16H[1K t.w = w[56;8H[1K } else {[29;17H[30d[31d[32d[57d[28S[29;17Hfmt.Printf("ERROR: tee created spontainiously?!\r\n")[30;9H}[31dt.mu.Unlock()[32d}[34dfunc command(raw bool, w *MessengerWriter, args ...string) {[35;9Hswitch args[0] {[36;9Hcase "help":[37;17Hif raw {[38dreturn[39;17H}[40dfmt.Printf("Commands:\n")[41;17Hfmt.Printf("  excl   - detach all other clients\n")[42;17Hfmt.Printf("  env    - display environment variables\n")[43;17Hfmt.Printf("  list   - list all clients\n")[44;17Hfmt.Printf("  save   - save buffer to FILE\n")[45;17Hfmt.Printf("  setenv - list all clients\n")[46;17Hfmt.Printf("  ssh    - forward SSH_AUTH_SOCK\n")[47;9Hcase "list":[48;17Hif raw {[49dw.Send(listMessage, nil)[50;17H}[51;9Hcase "excl":[52;17Hif raw {[53dw.Send(exclusiveMessage, nil)[54;17H}[55;9Hcase "getenv", "env":[56;17Hif raw {[32d[31;8H[A[A[A[57d(B[0;7m^I isn't a vi command[28;8H(B[m[?5h[?5l[57d[J[28;8H[A[57dCopying file for recovery...[J[27;8H7[28;56r8[28dM[1;57r[28;1H[28;9H[28;17Ht.path = path[57d:wqWriting...[Jmain.go: 475 lines, 9358 characters[?1l>
[56;36H.[57d[57;1H[?1049l[?1l>MN Linux => go build
MN Linux => clear
[3;J[H[2JMN Linux => exit`
