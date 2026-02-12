package main

import (
	"Skyfield1888/WebMine/backend"
	filesdownload "Skyfield1888/WebMine/backend/files_download"
	"fmt"
	"log"
	"net/http"
)

func main() {
	backend.DecodeConfig()

	err := filesdownload.CheckFolderStructure()
	if err != nil {
		log.Fatal(err)
	}

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
	http.HandleFunc("/chart/players", backend.PlayerLineHandler)

	port := ":"+backend.SavedAppConfig.WebAppConfig.Port

	fmt.Printf("Server listening on %s\n", port)
	log.Fatal(http.ListenAndServe(port, nil))
}
