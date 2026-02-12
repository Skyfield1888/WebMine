package main

import (
	"Skyfield1888/WebMine/backend"
	"fmt"
	"log"
	"net/http"
)

func main() {
	backend.DecodeConfig()
	log.Println("Starting Minecraft server WebSocket controller")
	//Console
	http.HandleFunc("/console/ws", backend.WsHandler)
	http.HandleFunc("/console/start", backend.StartHandler)
	http.HandleFunc("/console/stop", backend.StopHandler)
	http.HandleFunc("/console/restart", backend.RestartHandler)
	http.HandleFunc("/console/view", backend.ConsoleHandler)
	//Properties Handeler
	http.HandleFunc("/properties/set", backend.ChangePropertiesHandler)
	http.HandleFunc("/properties/view", backend.PropertiesTableHandler)

	//App Setting Handeler
	http.HandleFunc("/settings/set", backend.ChangeAppSettingsHandler)
	http.HandleFunc("/settings/view", backend.AppSettingsTableHandler)

	// Pages handler
	http.HandleFunc("/current_page", backend.CurrentPageHandler)

	// main website handeler
	http.Handle("/templates/", http.FileServer(http.Dir("frontend")))
	http.Handle("/", http.FileServer(http.Dir("frontend/static")))

	//Charts Handelers
	http.HandleFunc("/chart/cpu", backend.CpuLineHandler)
	http.HandleFunc("/chart/ram", backend.RamLineHandler)

	fmt.Println("Server listening on :8082")
	log.Fatal(http.ListenAndServe(":"+backend.SavedAppConfig.WebAppConfig.Port, nil))
}
