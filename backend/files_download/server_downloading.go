package filesdownload

import (
	"Skyfield1888/WebMine/backend"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const DEFAULT_VERSION_MANIFEST_URL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
const FALLBACK_VERSION_MANIFEST_URL = "https://piston-meta.mojang.com/mc/game/version_manifest.json"

type ServerType int

const (
	Vanilla ServerType = iota
	// Fabric
	// Quilt
	// Forge
	// NeoForge
	// Paper
	// Folia
	// Above to be implemented
)

type MojangVersionsManifest struct {
	LatestVersions struct {
		Release string
		Snapshot string
	} `json:"latest"`

	Versions []struct{
		Id string
		Url string
		VersionType string `json:"type"`
		ReleaseTime time.Time
		Sha1 string
	} `json:"versions"`
}

type MojangVersionManifest struct {
	Downloads struct {
		Server struct {
			Url string
		}
	}

	Id string

	JavaVersion struct {
		Component string
		MajorVersion int
	}
}

func (manifest *MojangVersionsManifest) Populate(url string) error {
	manifestData, err := http.Get(url)
	if err != nil { return err }
	
	defer manifestData.Body.Close()

	manifestBody, err := io.ReadAll(manifestData.Body)
	if err != nil { return err }
	
	// log.Print(string(manifestBody))

	err = json.Unmarshal(manifestBody, &manifest)
	if err != nil { return err }
	
	return nil
}

func GetVersionUrl(version string, manifest MojangVersionsManifest) (string, error) {
	log.Print(manifest.Versions)
	for _, minecraftVersion := range manifest.Versions {
		if minecraftVersion.Id == version {
			return minecraftVersion.Url, nil
		}
	}
	return "", fmt.Errorf("Version %s is nowhere to be found.", version)
}

func GetVersionInfo(url string) (MojangVersionManifest, error) {
	result := MojangVersionManifest{}
	versionJson, err := http.Get(url)
	if err != nil { return MojangVersionManifest{}, err }

	defer versionJson.Body.Close()

	versionData, err := io.ReadAll(versionJson.Body)
	err = json.Unmarshal(versionData, &result)
	if err != nil { return MojangVersionManifest{}, err }

	return result, nil
}

func DownloadVanillaServer(path string, version string) error {
	manifest := MojangVersionsManifest{}
	err := manifest.Populate(DEFAULT_VERSION_MANIFEST_URL)
	if err != nil { return err }

	versionUrl, err := GetVersionUrl(version, manifest)
	if err != nil { return err }

	versionInfo, err := GetVersionInfo(versionUrl)
	if err != nil {return err}

	serverUrl := versionInfo.Downloads.Server.Url

	out, err := os.Create(fmt.Sprintf("%sserver.jar", path))
	if err != nil {return err}

	defer out.Close()

	response, err := http.Get(serverUrl)
	if err != nil {return err}

	defer response.Body.Close()

	_, err = io.Copy(out, response.Body)
	if err != nil {return err}

	return nil
}

func CheckFolderStructure() error {
	_, err := os.Stat(backend.SavedAppConfig.MinecraftServerConfig.PathToMcServers)
	if errors.Is(err, os.ErrNotExist) {
		return os.Mkdir(backend.SavedAppConfig.MinecraftServerConfig.PathToMcServers, 0755)
	}
	return err
}