// viewer/main.go
//
// Runs on the machine that wants to *view/control* the shared terminal.
// You paste in the host's offer code, this prints back an answer code,
// and once the data channel opens your local stdin/stdout are wired
// directly to the remote pty.
package main

import (
	"bufio"
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/pion/webrtc/v4"
	"golang.org/x/term"
)

func main() {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
	})
	if err != nil {
		log.Fatalf("failed to create peer connection: %v", err)
	}
	defer pc.Close()

	var dataChan *webrtc.DataChannel
	connected := make(chan struct{})
	done := make(chan struct{})

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		dataChan = dc
		dc.OnOpen(func() {
			select {
			case <-connected:
			default:
				close(connected)
			}
		})
		dc.OnClose(func() {
			select {
			case <-done:
			default:
				close(done)
			}
		})
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			os.Stdout.Write(msg.Data)
		})
	})

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state == webrtc.ICEConnectionStateDisconnected || state == webrtc.ICEConnectionStateFailed || state == webrtc.ICEConnectionStateClosed {
			select {
			case <-done:
			default:
				close(done)
			}
		}
	})

	fmt.Print("Paste the host's code (or press Enter to use clipboard):\n> ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = cleanInput(line)
	if line == "" {
		line = cleanInput(readFromClipboard())
		if line != "" {
			fmt.Println("📋 Using code from system clipboard.")
		}
	}

	var offer webrtc.SessionDescription
	decode(line, &offer)
	if err := pc.SetRemoteDescription(offer); err != nil {
		log.Fatalf("failed to set remote description: %v", err)
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		log.Fatalf("failed to create answer: %v", err)
	}
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		log.Fatalf("failed to set local description: %v", err)
	}
	<-gatherComplete

	answerCode := encode(pc.LocalDescription())
	fmt.Println("\n=== Send this code back to the host ===")
	fmt.Println(answerCode)
	fmt.Println("=== end code ===")
	if copyToClipboard(answerCode) {
		fmt.Println("👉 Code copied to system clipboard automatically!")
	}

	fmt.Println("\nConnecting to host...")
	select {
	case <-connected:
	case <-done:
		log.Fatal("failed to connect to host")
	}

	// Put local terminal into raw mode so keystrokes go directly to remote shell
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err == nil {
		defer term.Restore(int(os.Stdin.Fd()), oldState)
	} else {
		log.Printf("warning: could not set raw mode: %v", err)
	}

	// Forward local keyboard input to remote DataChannel
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 && dataChan != nil {
				// Check for Ctrl+] (ASCII 29 / 0x1D) to instantly detach
				for i := 0; i < n; i++ {
					if buf[i] == 0x1d {
						fmt.Print("\r\n\033[1;33m[MiniShare] Detached via Ctrl+].\033[0m\r\n")
						select {
						case <-done:
						default:
							close(done)
						}
						return
					}
				}

				if sendErr := dataChan.Send(buf[:n]); sendErr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		select {
		case <-done:
		default:
			close(done)
		}
	}()

	<-done
	fmt.Println("\r\n\033[1;31m[MiniShare] Disconnected from remote session. Returned to local terminal.\033[0m")
}

func encode(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		log.Fatalf("marshal error: %v", err)
	}
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		return base64.StdEncoding.EncodeToString(b)
	}
	w.Write(b)
	w.Close()
	return base64.RawURLEncoding.EncodeToString(buf.Bytes())
}

func decode(s string, v interface{}) {
	s = cleanInput(s)
	if s == "" {
		log.Fatalf("no input code provided")
	}

	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		b, err = base64.StdEncoding.DecodeString(s)
		if err != nil {
			log.Fatalf("invalid code encoding: %v", err)
		}
	}

	r := flate.NewReader(bytes.NewReader(b))
	decompressed, err := io.ReadAll(r)
	r.Close()
	if err == nil && len(decompressed) > 0 {
		b = decompressed
	}

	if err := json.Unmarshal(b, v); err != nil {
		log.Fatalf("invalid code contents: %v", err)
	}
}

func cleanInput(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "\"", "")
	s = strings.ReplaceAll(s, "'", "")
	return s
}

func copyToClipboard(text string) bool {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("pbcopy")
	} else if runtime.GOOS == "linux" {
		cmd = exec.Command("xclip", "-selection", "clipboard")
	} else if runtime.GOOS == "windows" {
		cmd = exec.Command("clip")
	} else {
		return false
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run() == nil
}

func readFromClipboard() string {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("pbpaste")
	} else if runtime.GOOS == "linux" {
		cmd = exec.Command("xclip", "-selection", "clipboard", "-o")
	} else {
		return ""
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}
