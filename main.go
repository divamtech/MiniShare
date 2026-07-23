// host/main.go
//
// Runs on the machine whose terminal you want to share.
// Spawns a real shell in a pty, opens a WebRTC PeerConnection with a single
// DataChannel, and pipes the pty's stdin/stdout over that channel.
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
	"time"

	"github.com/creack/pty"
	"github.com/pion/webrtc/v4"
)

func main() {
	// 1. Spawn a real shell attached to a pseudo-terminal.
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	cmd := exec.Command(shell)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Fatalf("failed to start pty: %v", err)
	}
	defer ptmx.Close()

	// 2. Set up a WebRTC PeerConnection.
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
	})
	if err != nil {
		log.Fatalf("failed to create peer connection: %v", err)
	}
	defer pc.Close()

	done := make(chan struct{})

	// 3. The data channel is the actual pipe the terminal bytes travel over.
	dc, err := pc.CreateDataChannel("terminal", nil)
	if err != nil {
		log.Fatalf("failed to create data channel: %v", err)
	}

	dc.OnOpen(func() {
		log.Println("data channel open — session live")
		
		// Send initial welcome banner to viewer
		hostname, _ := os.Hostname()
		banner := fmt.Sprintf("\r\n\033[1;32m┌─────────────────────────────────────────────────────────────┐\033[0m\r\n"+
			"\033[1;32m│  ⚡ CONNECTED TO REMOTE HOST: %-29s │\033[0m\r\n"+
			"\033[1;32m│  OS: %-9s | Shell: %-25s │\033[0m\r\n"+
			"\033[1;32m│  Exit: Type 'exit' or press 'Ctrl+]' to detach             │\033[0m\r\n"+
			"\033[1;32m└─────────────────────────────────────────────────────────────┘\033[0m\r\n\r\n",
			hostname, runtime.GOOS, shell)
		_ = dc.Send([]byte(banner))

		// pty -> remote
		go func() {
			buf := make([]byte, 4096)
			for {
				n, err := ptmx.Read(buf)
				if n > 0 {
					if sendErr := dc.Send(buf[:n]); sendErr != nil {
						break
					}
				}
				if err != nil {
					if err != io.EOF {
						log.Printf("pty read error: %v", err)
					}
					break
				}
			}
			select {
			case <-done:
			default:
				close(done)
			}
		}()
	})

	// remote -> pty (with live command logging on Host console)
	var cmdBuffer bytes.Buffer
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		// Log typed command to host logs when Enter (\r or \n) is pressed
		for _, b := range msg.Data {
			if b == '\r' || b == '\n' {
				if typedCmd := strings.TrimSpace(cmdBuffer.String()); typedCmd != "" {
					log.Printf("⌨️  [Viewer Command Executed]: %s", typedCmd)
				}
				cmdBuffer.Reset()
			} else if b == 127 || b == 8 { // Backspace
				if cmdBuffer.Len() > 0 {
					cmdBuffer.Truncate(cmdBuffer.Len() - 1)
				}
			} else if b >= 32 && b <= 126 { // Printable ASCII characters
				cmdBuffer.WriteByte(b)
			}
		}

		// Write incoming bytes directly to pseudo-terminal stdin
		_, _ = ptmx.Write(msg.Data)
	})

	dc.OnClose(func() {
		log.Println("data channel closed")
		select {
		case <-done:
		default:
			close(done)
		}
	})

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state == webrtc.ICEConnectionStateDisconnected || state == webrtc.ICEConnectionStateFailed || state == webrtc.ICEConnectionStateClosed {
			log.Printf("connection state: %s", state.String())
			select {
			case <-done:
			default:
				close(done)
			}
		}
	})

	// 4. Build the offer and wait for ICE gathering to finish
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		log.Fatalf("failed to create offer: %v", err)
	}
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(offer); err != nil {
		log.Fatalf("failed to set local description: %v", err)
	}
	<-gatherComplete

	offerCode := encode(pc.LocalDescription())
	fmt.Println("\n=== Share this code with the viewer ===")
	fmt.Println(offerCode)
	fmt.Println("=== end code ===")
	if copyToClipboard(offerCode) {
		fmt.Println("👉 Code copied to system clipboard automatically!")
	}

	fmt.Print("\nPaste the code from the viewer (or press Enter to use clipboard):\n> ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = cleanInput(line)
	if line == "" {
		line = cleanInput(readFromClipboard())
		if line != "" {
			fmt.Println("📋 Using code from system clipboard.")
		}
	}

	var answer webrtc.SessionDescription
	decode(line, &answer)
	if err := pc.SetRemoteDescription(answer); err != nil {
		log.Fatalf("failed to set remote description: %v", err)
	}

	log.Println("waiting for viewer to connect...")
	<-done
	log.Println("session ended")
	time.Sleep(100 * time.Millisecond)
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
