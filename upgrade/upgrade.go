// +build !cgo
// +build upgrade,ignore

package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const MC_VERSION = "v1.3.4"

type GithubRes []Item

type Item struct {
	TagName string `json:"tag_name"`
	Assets  Assets
}

type Assets []struct {
	Name               string
	BrowserDownloadUrl string `json:"browser_download_url"`
}

func getAmagationZipUrl() string {
	// https://api.github.com/repos/utelle/SQLite3MultipleCiphers/releases
	//jq -r ".[].assets[] | select(.name | contains(\"$(version)-amalgamation\")) | .created_at |= fromdateiso8601 | .browser_download_url" | head -1 | wget -O $@ -i -

	resp, err := http.Get("https://api.github.com/repos/utelle/SQLite3MultipleCiphers/releases")
	if err != nil {
		log.Fatal("Could not get url: ", err.Error())
	}
	defer resp.Body.Close()

	items := GithubRes{}
	bytes, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(bytes, &items)

	var asset *Assets

	for _, v := range items {
		if v.TagName == MC_VERSION {
			asset = &v.Assets
			break
		}
	}

	if asset == nil {
		log.Fatal("Version not found !")
	}

	var downloadUrl string
	for _, it := range *asset {
		if strings.Contains(it.Name, "-amalgamation") {
			downloadUrl = it.BrowserDownloadUrl
		}
	}

	return downloadUrl
}

func download() (url string, content []byte, err error) {

	url = getAmagationZipUrl()

	fmt.Printf("Downloading %v\n", url)
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}

	// Ready Body Content
	content, err = ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return "", nil, err
	}

	return url, content, nil
}

func main() {
	fmt.Println("Go-SQLite3 Upgrade Tool")

	// Download Amalgamation
	_, amalgamation, err := download()
	if err != nil {
		fmt.Println("Failed to download: sqlite-amalgamation; %s", err)
	}

	// Create Amalgamation Zip Reader
	rAmalgamation, err := zip.NewReader(bytes.NewReader(amalgamation), int64(len(amalgamation)))
	if err != nil {
		log.Fatal(err)
	}

	// Extract Amalgamation
	for _, zf := range rAmalgamation.File {
		var f *os.File
		switch path.Base(zf.Name) {
		case "sqlite3mc_amalgamation.c":
			f, err = os.Create("sqlite3-binding.c")
		case "sqlite3mc_amalgamation.h":
			f, err = os.Create("sqlite3-binding.h")
		case "sqlite3ext.h":
			f, err = os.Create("sqlite3ext.h")
		default:
			continue
		}
		if err != nil {
			log.Fatal(err)
		}
		zr, err := zf.Open()
		if err != nil {
			log.Fatal(err)
		}

		_, err = io.WriteString(f, "#ifndef USE_LIBSQLITE3\n")
		if err != nil {
			zr.Close()
			f.Close()
			log.Fatal(err)
		}
		scanner := bufio.NewScanner(zr)
		for scanner.Scan() {
			text := scanner.Text()
			if text == `#include "sqlite3.h"` {
				text = `#include "sqlite3-binding.h"
#ifdef __clang__
#define assert(condition) ((void)0)
#endif
`
			}
			_, err = fmt.Fprintln(f, text)
			if err != nil {
				break
			}
		}
		err = scanner.Err()
		if err != nil {
			zr.Close()
			f.Close()
			log.Fatal(err)
		}
		_, err = io.WriteString(f, "#else // USE_LIBSQLITE3\n // If users really want to link against the system sqlite3 we\n// need to make this file a noop.\n #endif")
		if err != nil {
			zr.Close()
			f.Close()
			log.Fatal(err)
		}
		zr.Close()
		f.Close()
		fmt.Printf("Extracted: %v\n", filepath.Base(f.Name()))
	}

	os.Exit(0)
}
