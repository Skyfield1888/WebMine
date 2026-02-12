package backend

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os/exec"
	"regexp"
	"sync"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/render"
	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/process"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var mcServer = &McServer{}

type McServer struct {
	cmd       *exec.Cmd
	stdin     *bufio.Writer
	mu        sync.Mutex
	active    bool
	ws        *websocket.Conn
	lastStats Stats
	players   Players
}

type Stats struct {
	cpu []float64
	ram []uint64
}
type Players struct {
	playersNames   []string
	playersNumbers []int
}

type HTMXMessage struct {
	Command string            `json:"command"`
	Headers map[string]string `json:"HEADERS"`
}

func logMessage(ws *websocket.Conn, msgType string, text string) {
	if ws == nil {
		return
	}
	msg, _ := json.Marshal(map[string]string{
		"type": msgType,
		"text": text,
	})
	ws.WriteMessage(websocket.TextMessage, msg)
}

func (mc *McServer) Start() error {
	fmt.Println("\nAttempting to start Minecraft server")
	mc.mu.Lock()
	if mc.active {
		mc.mu.Unlock()
		fmt.Println("Server start failed: already running")
		if mc.ws != nil {
			logMessage(mcServer.ws, "log", "Server already running!")
		}
		return fmt.Errorf("\nserver already running")
	}
	mc.active = true
	mc.mu.Unlock()

	command := "java"
	arg1 := "-Xmx" + SavedAppConfig.MinecraftServerConfig.MaxAllowedRam
	arg2 := "-Xms" + SavedAppConfig.MinecraftServerConfig.MinAllowedRam
	arg3 := "-jar"
	arg4 := SavedAppConfig.MinecraftServerConfig.ServerJarName
	arg5 := SavedAppConfig.MinecraftServerConfig.OthersCommandArguments

	cmd := exec.Command(command, arg1, arg2, arg3, arg4, arg5)
	cmd.Dir = SavedAppConfig.MinecraftServerConfig.PathToMcServers
	mc.cmd = cmd

	fmt.Printf("\nExecuting command: %s %s %s %s in directory %s", command, arg1, arg2, arg3, SavedAppConfig.MinecraftServerConfig.PathToMcServers)

	// Pipes from cmd
	stdout, err := mc.cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("\nError getting stdout pipe: %v", err)
		mc.active = false
		return err
	}

	stderr, err := mc.cmd.StderrPipe()
	if err != nil {
		fmt.Printf("\nError getting stderr pipe: %v", err)
		mc.active = false
		return err
	}

	stdin, err := mc.cmd.StdinPipe()
	if err != nil {
		fmt.Printf("\nError getting stdin pipe: %v", err)
		mc.active = false
		return err
	}
	mc.stdin = bufio.NewWriter(stdin)

	// Run the Command
	if err := mc.cmd.Start(); err != nil {
		fmt.Printf("\nError starting server: %v", err)
		mc.active = false
		return err
	}
	pid := int32(cmd.Process.Pid)

	fmt.Println("Minecraft server process started successfully")

	go func() {
		proc, _ := process.NewProcess(pid)
		proc.CPUPercent()
		time.Sleep(100 * time.Millisecond)
		for {

			memInfo, _ := proc.MemoryInfo()

			cpuPercent, _ := proc.CPUPercent()
			numCores, _ := cpu.Counts(true)
			normalizedCpuPercent := cpuPercent / float64(numCores)
			mbOfRam := memInfo.RSS / 1024 / 1024

			if mc.ws != nil {
				mc.lastStats.cpu = append(mc.lastStats.cpu, normalizedCpuPercent)
				if len(mc.lastStats.cpu) > 30 {
					mc.lastStats.cpu = mc.lastStats.cpu[1:]
				}
				mc.lastStats.ram = append(mc.lastStats.ram, mbOfRam)
				if len(mc.lastStats.ram) > 30 {
					mc.lastStats.ram = mc.lastStats.ram[1:]
				}
				stats, _ := json.Marshal(map[string]interface{}{
					"type":   "stats",
					"cpu":    normalizedCpuPercent,
					"ram_mb": mbOfRam,
				})
				mc.ws.WriteMessage(websocket.TextMessage, stats)
			}

			time.Sleep(2 * time.Second)
		}
	}()

	// Pipe stdout to websocket
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			text := scanner.Text()
			var logLevel = regexp.MustCompile(`\[Server thread/(\w+)\]`)

			var playerJoin = regexp.MustCompile(`([\w\-.]+) joined the game`)
			var playerLeave = regexp.MustCompile(`([\w\-.]+) left the game`)

			if match := logLevel.FindStringSubmatch(text); match != nil {
				switch match[1] {
				case "ERROR":
					logMessage(mc.ws, "error", text)
				case "WARN":
					logMessage(mc.ws, "warn", text)
				default:
					logMessage(mc.ws, "log", text)
				}
			} else {
				logMessage(mc.ws, "log", text)
			}

			if match := playerJoin.FindStringSubmatch(text); match != nil {
				player := match[1]
				mc.players.playersNames = append(mc.players.playersNames, player)
				playerNumber := len(mc.players.playersNames)
				if len(mc.players.playersNames) >= 0 {
					playerNumber += 1
				} else {
					playerNumber = 0
				}
				mc.players.playersNumbers = append(mc.players.playersNumbers, playerNumber)
				stats, _ := json.Marshal(map[string]interface{}{
					"type":   "player",
					"name":   player,
					"number": playerNumber,
				})
				mc.ws.WriteMessage(websocket.TextMessage, stats)
				fmt.Println(mc.players)
			}

			if match := playerLeave.FindStringSubmatch(text); match != nil {
				player := match[1]
				for i, p := range mc.players.playersNames {
					if p == player {
						mc.players.playersNames = append(mc.players.playersNames[:i], mc.players.playersNames[i+1:]...)
						playerNumber := len(mc.players.playersNames)
						if len(mc.players.playersNames) >= 0 {
							playerNumber += 1
						} else {
							playerNumber = 0
						}
						mc.players.playersNumbers = append(mc.players.playersNumbers, len(mc.players.playersNames))
						break
					}
				}
				logMessage(mc.ws, "player", player)
				fmt.Println(mc.players)
			}

		}
		if err := scanner.Err(); err != nil {
			fmt.Printf("\nError reading stdout: %v", err)
		}
	}()

	// Pipe stderr to websocket
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			text := scanner.Text()
			fmt.Printf("\n[MC-STDERR] %s", text)
			logMessage(mc.ws, "error", text) // "error" type so frontend can color it red
		}
		if err := scanner.Err(); err != nil {
			fmt.Printf("\nError reading stderr: %v", err)
		}
	}()

	// Wait for process to finish
	go func() {
		err := mc.cmd.Wait()
		mc.mu.Lock()
		mc.active = false
		mc.mu.Unlock()

		if err != nil {
			fmt.Printf("\nServer process exited with error: %v", err)
			logMessage(mc.ws, "stopped", "Server stopped with error: "+err.Error())
		} else {
			fmt.Println("Server process exited normally")
			logMessage(mc.ws, "stopped", "Server stopped")
		}
	}()

	return nil
}

func (mc *McServer) Stop() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if !mc.active {
		fmt.Println("Cannot stop server: not running")
		return fmt.Errorf("\nserver not running")
	}

	fmt.Println("Stopping Minecraft server...")

	_, err := mc.stdin.WriteString("stop\n")
	if err != nil {
		fmt.Printf("\nError writing stop command: %v", err)
		return err
	}

	if err := mc.stdin.Flush(); err != nil {
		fmt.Printf("\nError flushing stop command: %v", err)
		return err
	}

	fmt.Println("Stop command sent successfully")
	return nil
}

func (mc *McServer) Restart() error {
	mc.mu.Lock()
	if !mc.active {
		mc.mu.Unlock()
		fmt.Println("Server not running, starting ...")
		return mc.Start()
	}
	mc.mu.Unlock()

	fmt.Println("Restarting Minecraft server...")

	if err := mc.Stop(); err != nil {
		fmt.Printf("\nFailed to stop server: %v", err)
		return err
	}

	if mc.cmd != nil && mc.cmd.Process != nil {
		mc.cmd.Wait()
		fmt.Println("Server process has fully stopped")
	}

	if err := mc.Start(); err != nil {
		fmt.Printf("\nFailed to start server: %v", err)
		return err
	}

	fmt.Println("Minecraft server restarted successfully")
	return nil
}

func (mc *McServer) SendCommand(command string) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if !mc.active {
		fmt.Printf("\nCannot send command '%s': server not running", command)
		return fmt.Errorf("\nserver not running")
	}

	fmt.Printf("\nSending command to MC server: %s", command)

	// write command for minecraft input
	_, err := mc.stdin.WriteString(command + "\n")
	if err != nil {
		fmt.Printf("\nError writing command: %v", err)
		return err
	}

	// send now
	if err := mc.stdin.Flush(); err != nil {
		fmt.Printf("\nError flushing command: %v", err)
		return err
	}

	fmt.Printf("\nCommand sent successfully: %s", command)
	return nil
}

func (mc *McServer) SetWebSocket(ws *websocket.Conn) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.ws = ws
}

func WsHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("\nNew WebSocket connection from %s", r.RemoteAddr)

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("\nWebSocket upgrade error from %s: %v", r.RemoteAddr, err)
		return
	}
	defer func() {
		ws.Close()
		fmt.Printf("\nWebSocket connection closed for %s", r.RemoteAddr)
	}()

	mcServer.SetWebSocket(ws)

	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			fmt.Printf("\nWebSocket read error from %s: %v", r.RemoteAddr, err)
			break
		}

		var HTMXMessage HTMXMessage
		jsonErr := json.Unmarshal(message, &HTMXMessage)
		if jsonErr != nil {
			fmt.Printf("\nError decoding JSON from %s: %v", r.RemoteAddr, jsonErr)
			continue // Skip this message and continue listening
		}

		command := HTMXMessage.Command
		fmt.Printf("\nReceived message from %s: %s", r.RemoteAddr, command)

		if err := mcServer.SendCommand(command); err != nil {
			fmt.Printf("\nFailed to send command: %v", err)
			ws.WriteMessage(websocket.TextMessage, []byte("Error: "+err.Error()))
		}
	}
}

func StartHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("\nStart server request from %s", r.RemoteAddr)

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := mcServer.Start(); err != nil {
		fmt.Printf("\nFailed to start server: %v", err)
		http.Error(w, fmt.Sprintf("Error: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Minecraft server started",
	})
}

func StopHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("\nStop server request from %s", r.RemoteAddr)

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := mcServer.Stop(); err != nil {
		fmt.Printf("\nFailed to stop server: %v", err)
		http.Error(w, fmt.Sprintf("Error: %s", err.Error()), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Stop command sent to Minecraft server",
	})
}
func RestartHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("\nRestart server request from %s", r.RemoteAddr)

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := mcServer.Restart(); err != nil {
		fmt.Printf("\nFailed to Restart server: %v", err)
		http.Error(w, fmt.Sprintf("\nError: %s", err.Error()), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Restart command sent to Minecraft server",
	})
}

func CpuLineHandler(w http.ResponseWriter, r *http.Request) {
	history := make([]float64, len(mcServer.lastStats.cpu))
	copy(history, mcServer.lastStats.cpu)
	items := make([]opts.LineData, 30)
	xAxis := make([]string, 30)

	for i := 0; i < 30; i++ {
		offset := 30 - len(history)
		if i < offset {
			items[i] = opts.LineData{Value: 0}
		} else {
			items[i] = opts.LineData{Value: history[i-offset]}
		}
		xAxis[i] = fmt.Sprintf("-%ds", (30-i)*2)
	}

	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithXAxisOpts(opts.XAxis{
			AxisLabel: &opts.AxisLabel{Show: opts.Bool(false)},
		}),
		charts.WithYAxisOpts(opts.YAxis{
			Min:       "0",
			Max:       "100",
			AxisLabel: &opts.AxisLabel{Show: opts.Bool(false)},
		}),
		charts.WithInitializationOpts(opts.Initialization{
			Width:     "250px",
			Height:    "150px",
			PageTitle: " ",
		}),
		charts.WithAnimation(false),
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(false)}),
		charts.WithGridOpts(opts.Grid{
			Top:          "0%",
			Bottom:       "2.5%",
			Left:         "0%",
			Right:        "5%",
			ContainLabel: opts.Bool(true),
		}),
	)

	line.SetXAxis(xAxis).
		AddSeries("CPU", items).
		SetSeriesOptions(
			charts.WithAreaStyleOpts(opts.AreaStyle{Opacity: opts.Float(0.4)}),
			charts.WithLabelOpts(opts.Label{Show: opts.Bool(false)}),
		)

	line.Render(w)

	var buf bytes.Buffer
	renderer := render.NewChartRender(line, line.Validate)
	renderer.Render(&buf)

	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
}

func RamLineHandler(w http.ResponseWriter, r *http.Request) {
	history := make([]uint64, len(mcServer.lastStats.ram))
	copy(history, mcServer.lastStats.ram)
	items := make([]opts.LineData, 30)
	xAxis := make([]string, 30)

	for i := 0; i < 30; i++ {
		offset := 30 - len(history)
		if i < offset {
			items[i] = opts.LineData{Value: 0}
		} else {
			items[i] = opts.LineData{Value: history[i-offset]}
		}
		xAxis[i] = fmt.Sprintf("-%ds", (30-i)*2)
	}

	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithXAxisOpts(opts.XAxis{
			AxisLabel: &opts.AxisLabel{Show: opts.Bool(false)},
		}),
		charts.WithYAxisOpts(opts.YAxis{
			Min:       "0",
			AxisLabel: &opts.AxisLabel{Show: opts.Bool(false)},
		}),
		charts.WithInitializationOpts(opts.Initialization{
			Width:     "250px",
			Height:    "150px",
			PageTitle: " ",
		}),
		charts.WithAnimation(false),
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(false)}),
		charts.WithGridOpts(opts.Grid{
			Top:          "0%",
			Bottom:       "2.5%",
			Left:         "0%",
			Right:        "5%",
			ContainLabel: opts.Bool(true),
		}),
	)
	line.SetXAxis(xAxis).
		AddSeries("RAM", items).
		SetSeriesOptions(
			charts.WithAreaStyleOpts(opts.AreaStyle{Opacity: opts.Float(0.4)}),
			charts.WithLabelOpts(opts.Label{Show: opts.Bool(false)}),
		)

	var buf bytes.Buffer
	renderer := render.NewChartRender(line, line.Validate)
	renderer.Render(&buf)

	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
}

func PlayerLineHandler(w http.ResponseWriter, r *http.Request) {
	history := make([]int, len(mcServer.players.playersNames))
	copy(history, mcServer.players.playersNumbers)
	items := make([]opts.LineData, 30)
	xAxis := make([]string, 30)

	for i := 0; i < 30; i++ {
		offset := 30 - len(history)
		if i < offset {
			items[i] = opts.LineData{Value: 0}
		} else {
			items[i] = opts.LineData{Value: history[i-offset]}
		}
		xAxis[i] = fmt.Sprintf("-%ds", (30-i)*2)
	}

	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithXAxisOpts(opts.XAxis{
			AxisLabel: &opts.AxisLabel{Show: opts.Bool(false)},
		}),
		charts.WithYAxisOpts(opts.YAxis{
			Min:       "0",
			Max:       "100",
			AxisLabel: &opts.AxisLabel{Show: opts.Bool(false)},
		}),
		charts.WithInitializationOpts(opts.Initialization{
			Width:     "250px",
			Height:    "150px",
			PageTitle: " ",
		}),
		charts.WithAnimation(false),
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(false)}),
		charts.WithGridOpts(opts.Grid{
			Top:          "0%",
			Bottom:       "2.5%",
			Left:         "0%",
			Right:        "5%",
			ContainLabel: opts.Bool(true),
		}),
	)

	line.SetXAxis(xAxis).
		AddSeries("Players", items).
		SetSeriesOptions(
			charts.WithAreaStyleOpts(opts.AreaStyle{Opacity: opts.Float(0.4)}),
			charts.WithLabelOpts(opts.Label{Show: opts.Bool(false)}),
		)

	line.Render(w)

	var buf bytes.Buffer
	renderer := render.NewChartRender(line, line.Validate)
	renderer.Render(&buf)

	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
}

func ConsoleHandler(w http.ResponseWriter, r *http.Request) {
	var consoleTemplate = template.Must(template.New("console.html").ParseFiles("./frontend/templates/console.html"))

	consoleTemplate.ExecuteTemplate(w, "console.html", nil)
}
