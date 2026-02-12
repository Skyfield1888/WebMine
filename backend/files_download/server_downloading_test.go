package filesdownload

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"testing"
)

func CheckChecksum(filepath string, sha256Hash string) error {
	file, err := os.Open(filepath)
	if err != nil {
		log.Fatal(err)
	}

	defer file.Close()

	hasher := sha256.New()

	if _, err := io.Copy(hasher, file); err != nil {
		log.Fatal(err)
	}

	checksum := hasher.Sum(nil)

	if hex.EncodeToString(checksum) != sha256Hash {
		return fmt.Errorf("Checksum of %s does not match expected value", filepath)
	}
	return nil
}
func TestDownloadVanillaServer(t *testing.T) {
	err := DownloadVanillaServer("./test/", "1.21.10")
	if err != nil {
		t.Fatal(err)
	}
	ans := CheckChecksum("./test/server.jar", "5bb64dc47379903e8f288bd6a4b276e889075c5c0f4c0b714e958d835c1874e7")
	if ans != nil {
		t.Errorf("Downloaded server does not match reference hash")
	}
}