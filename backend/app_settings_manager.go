package backend

import (
	"html/template"
	"net/http"
	"os"
	"reflect"

	"github.com/BurntSushi/toml"
)

type AppConfig struct {
	MinecraftServerConfig MinecraftServerConfig
	WebAppConfig          WebAppConfig
}

type WebAppConfig struct {
	Port string
}

type MinecraftServerConfig struct {
	PathToMcServers        string
	MaxAllowedRam          string
	MinAllowedRam          string
	ServerJarName          string
	OthersCommandArguments string
}

var SavedAppConfig AppConfig

func Check(e error) {
	if e != nil {
		panic(e)
	}
}

func structToMap(s interface{}) map[string]map[string]string {
	result := make(map[string]map[string]string)
	v := reflect.ValueOf(s)
	t := reflect.TypeOf(s)

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i).String()
		result[field.Name] = map[string]string{
			"value": value,
			"type":  GetPropertyType(value),
		}
	}

	return result
}

func DecodeConfig() {
	_, err := toml.DecodeFile("./app_settings.toml", &SavedAppConfig)
	Check(err)
}

func EncodeConfig() {
	file, err := os.Create("./app_settings.toml")
	Check(err)
	defer file.Close()

	err = toml.NewEncoder(file).Encode(&SavedAppConfig)
	Check(err)
}

func ChangeAppSettingsHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	setting := r.FormValue("setting")
	value := r.FormValue("value")

	updated := false

	mcConfigValue := reflect.ValueOf(&SavedAppConfig.MinecraftServerConfig).Elem()
	mcConfigType := mcConfigValue.Type()
	for i := 0; i < mcConfigValue.NumField(); i++ {
		if mcConfigType.Field(i).Name == setting {
			field := mcConfigValue.Field(i)
			if field.CanSet() && field.Kind() == reflect.String {
				field.SetString(value)
				updated = true
				break
			}
		}
	}

	if !updated {
		webConfigValue := reflect.ValueOf(&SavedAppConfig.WebAppConfig).Elem()
		webConfigType := webConfigValue.Type()
		for i := 0; i < webConfigValue.NumField(); i++ {
			if webConfigType.Field(i).Name == setting {
				field := webConfigValue.Field(i)
				if field.CanSet() && field.Kind() == reflect.String {
					field.SetString(value)
					updated = true
					break
				}
			}
		}
	}

	EncodeConfig()
	http.Redirect(w, r, "/settings/view", http.StatusSeeOther)
}



func AppSettingsTableHandler(w http.ResponseWriter, r *http.Request) {
	var appSettingsTemplate = template.Must(template.New("app_settings.html").ParseFiles("./frontend/templates/app_settings.html"))

	w.Header().Set("Content-Type", "text/html")
	DecodeConfig()

	data := map[string]interface{}{
		"MinecraftServerConfig": structToMap(SavedAppConfig.MinecraftServerConfig),
		"WebAppConfig":          structToMap(SavedAppConfig.WebAppConfig),
	}

	appSettingsTemplate.ExecuteTemplate(w, "app_settings.html", data)
}
