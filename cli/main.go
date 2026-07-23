package main

import (
	"bufio"
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/creack/pty"
	"github.com/pion/webrtc/v4"
	"golang.org/x/term"
)

func main() {
	if len(os.Args) > 1 {
		cmd := strings.ToLower(os.Args[1])

		if cmd == "server" {
			HandleServerConfig(os.Args[2:])
			return
		}

		if cmd == "connect" {
			if len(os.Args) < 3 {
				fmt.Println("Usage: minishare connect <session-uuid>")
				os.Exit(1)
			}
			runViewer(os.Args[2])
			return
		}

		if cmd == "--help" || cmd == "-h" || cmd == "help" {
			printHelp()
			return
		}
	}

	// Default run mode: Host Session
	runHost()
}

func printHelp() {
	fmt.Println(`MiniShare CLI ⚡ - Real-time P2P Terminal Sharing

Usage:
  minishare                         Start Host session (creates session UUID)
  minishare connect <session-uuid>   Connect to a remote Host session
  minishare server <url>            Set custom signaling server URL
  minishare server reset            Reset signaling server to default
  minishare --manual                Run Host in manual copy-paste mode`)
}

// -------------------------------------------------------------------
// HOST MODE
// -------------------------------------------------------------------
func runHost() {
	cfg := LoadConfig()

	// FIRST LINE MUST BE INITIAL CONNECTION BANNER
	fmt.Printf("[MiniShare] Connecting to signaling server: %s\n", cfg.ServerURL)

	// Check manual fallback flag
	manualFlag := flag.Bool("manual", false, "Run in manual copy-paste mode")
	flag.CommandLine.Parse(os.Args[1:])

	// 1. Spawn a real shell attached to a pseudo-terminal
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

	// 2. Set up WebRTC PeerConnection
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
	})
	if err != nil {
		log.Fatalf("failed to create peer connection: %v", err)
	}
	defer pc.Close()

	done := make(chan struct{})

	// 3. Create DataChannel
	dc, err := pc.CreateDataChannel("terminal", nil)
	if err != nil {
		log.Fatalf("failed to create data channel: %v", err)
	}

	dc.OnOpen(func() {
		log.Println("Data channel open — P2P session live")
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
		for _, b := range msg.Data {
			if b == '\r' || b == '\n' {
				if typedCmd := strings.TrimSpace(cmdBuffer.String()); typedCmd != "" {
					log.Printf("⌨️  [Viewer Command Executed]: %s", typedCmd)
				}
				cmdBuffer.Reset()
			} else if b == 127 || b == 8 {
				if cmdBuffer.Len() > 0 {
					cmdBuffer.Truncate(cmdBuffer.Len() - 1)
				}
			} else if b >= 32 && b <= 126 {
				cmdBuffer.WriteByte(b)
			}
		}
		_, _ = ptmx.Write(msg.Data)
	})

	dc.OnClose(func() {
		log.Println("Data channel closed")
		select {
		case <-done:
		default:
			close(done)
		}
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

	// 4. Build offer and wait for ICE gathering
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		log.Fatalf("failed to create offer: %v", err)
	}
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(offer); err != nil {
		log.Fatalf("failed to set local description: %v", err)
	}
	<-gatherComplete

	offerCode := encodePayload(pc.LocalDescription())

	if *manualFlag {
		runManualHost(pc, offerCode, done)
		return
	}

	// Post offer to signaling server C
	serverURL := strings.TrimSuffix(cfg.ServerURL, "/")
	resp, err := postJSON(serverURL+"/api/session", map[string]string{"offer": offerCode})
	if err != nil {
		fmt.Printf("⚠️ Signaling server unavailable (%v). Falling back to manual mode...\n", err)
		runManualHost(pc, offerCode, done)
		return
	}

	var sessResp struct {
		UUID string `json:"uuid"`
	}
	_ = json.Unmarshal(resp, &sessResp)

	if sessResp.UUID == "" {
		fmt.Println("⚠️ Failed to obtain UUID from signaling server. Falling back to manual mode...")
		runManualHost(pc, offerCode, done)
		return
	}

	webLink := fmt.Sprintf("%s/#%s", serverURL, sessResp.UUID)
	fmt.Println("\n⚡ MiniShare Host Session Live")
	fmt.Printf("🔑 Session UUID: %s\n", sessResp.UUID)
	fmt.Printf("💻 Connect via CLI: minishare connect %s\n", sessResp.UUID)
	fmt.Printf("🌐 Connect via Web Browser: %s\n", webLink)
	copyToClipboard(sessResp.UUID)
	fmt.Println("👉 Session UUID copied to clipboard automatically!")

	fmt.Println("\nWaiting for peer to connect...")

	// Poll server for viewer answer
	go func() {
		for {
			time.Sleep(1 * time.Second)
			select {
			case <-done:
				return
			default:
			}

			ansResp, err := getJSON(fmt.Sprintf("%s/api/session/%s/answer", serverURL, sessResp.UUID))
			if err == nil {
				var ansData struct {
					Answer string `json:"answer"`
				}
				_ = json.Unmarshal(ansResp, &ansData)
				if ansData.Answer != "" {
					var answer webrtc.SessionDescription
					decodePayload(ansData.Answer, &answer)
					_ = pc.SetRemoteDescription(answer)
					break
				}
			}
		}
	}()

	<-done
	log.Println("Session ended")
	time.Sleep(100 * time.Millisecond)
}

func runManualHost(pc *webrtc.PeerConnection, offerCode string, done chan struct{}) {
	fmt.Println("\n=== Share this code with the viewer ===")
	fmt.Println(offerCode)
	fmt.Println("=== end code ===")
	copyToClipboard(offerCode)
	fmt.Println("👉 Code copied to system clipboard automatically!")

	fmt.Print("\nPaste the code from the viewer (or press Enter to use clipboard):\n> ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = cleanInput(line)
	if line == "" {
		line = cleanInput(readFromClipboard())
	}

	var answer webrtc.SessionDescription
	decodePayload(line, &answer)
	_ = pc.SetRemoteDescription(answer)

	log.Println("waiting for viewer to connect...")
	<-done
	log.Println("session ended")
}

// -------------------------------------------------------------------
// VIEWER MODE
// -------------------------------------------------------------------
func runViewer(uuid string) {
	cfg := LoadConfig()

	// FIRST LINE MUST BE INITIAL CONNECTION BANNER
	fmt.Printf("[MiniShare] Connecting to signaling server: %s\n", cfg.ServerURL)

	serverURL := strings.TrimSuffix(cfg.ServerURL, "/")
	resp, err := getJSON(fmt.Sprintf("%s/api/session/%s/offer", serverURL, uuid))
	if err != nil {
		log.Fatalf("failed to fetch session %s from signaling server: %v", uuid, err)
	}

	var offerData struct {
		Offer string `json:"offer"`
	}
	if err := json.Unmarshal(resp, &offerData); err != nil || offerData.Offer == "" {
		log.Fatalf("invalid session offer data from server")
	}

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

	var offer webrtc.SessionDescription
	decodePayload(offerData.Offer, &offer)
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

	encodedAnswer := encodePayload(pc.LocalDescription())
	_, err = postJSON(fmt.Sprintf("%s/api/session/%s/answer", serverURL, uuid), map[string]string{"answer": encodedAnswer})
	if err != nil {
		log.Fatalf("failed to post answer to signaling server: %v", err)
	}

	fmt.Println("Connecting to host P2P...")
	select {
	case <-connected:
	case <-done:
		log.Fatal("failed to connect to host")
	}

	// Terminal Raw mode setup
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err == nil {
		defer term.Restore(int(os.Stdin.Fd()), oldState)
	}

	// Keyboard input loop with Ctrl+] detach support
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 && dataChan != nil {
				for i := 0; i < n; i++ {
					if buf[i] == 0x1d { // Ctrl+]
						fmt.Print("\r\n\033[1;33m[MiniShare] Detached via Ctrl+].\033[0m\r\n")
						select {
						case <-done:
						default:
							close(done)
						}
						return
					}
				}
				_ = dataChan.Send(buf[:n])
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

// -------------------------------------------------------------------
// UTILITY & ENCODING FUNCTIONS
// -------------------------------------------------------------------
func encodePayload(v interface{}) string {
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

func decodePayload(s string, v interface{}) {
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

func postJSON(url string, payload interface{}) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}

func getJSON(url string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
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
