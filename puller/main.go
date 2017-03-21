package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sethgrid/pester"
)

var (
	garant     = "auth.docker.io"
	registry   = "registry-1.docker.io"
	httpClient = pester.NewExtendedClient(&http.Client{
		Timeout: time.Duration(30 * time.Second),
	})
)

type token struct {
	Token string
}

type layer struct {
	Digest string
	Size   uint32
}

type manifest struct {
	Layers []layer
}

func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Printf("%s took %s", name, elapsed)
}

func getJSON(url string, target interface{}) error {
	r, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(target)
}

func getToken(repository string) (token, error) {
	defer timeTrack(time.Now(), "getToken")
	url := fmt.Sprintf("https://%s/token?service=registry.docker.io&scope=repository:%s:*&scope=repository(plugin):%s:*", garant, repository, repository)
	t := new(token)
	err := getJSON(url, t)
	if err != nil {
		return token{}, err
	}
	return *t, nil
}

func getManifestV1Noop(repository string, tag string, token token) error {
	defer timeTrack(time.Now(), "getManifestV1Noop")
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repository, tag)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", "Bearer "+token.Token)
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v1+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	// Verify if the response was ok
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Server return non-200 status: %v", resp.Status)
	}
	return nil
}

func getManifestV2(repository string, tag string, token token) (manifest, error) {
	defer timeTrack(time.Now(), "getManifestV2")
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repository, tag)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return manifest{}, err
	}
	req.Header.Add("Authorization", "Bearer "+token.Token)
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return manifest{}, err
	}

	// Verify if the response was ok
	if resp.StatusCode != http.StatusOK {
		return manifest{}, fmt.Errorf("Server return non-200 status: %v", resp.Status)
	}

	m := new(manifest)
	err = json.NewDecoder(resp.Body).Decode(m)
	if err != nil {
		return manifest{}, err
	}
	return *m, nil
}

func getLayers(repository string, manifest manifest, token token) error {
	defer timeTrack(time.Now(), "getLayers")
	for _, l := range manifest.Layers {
		url := fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry, repository, l.Digest)
		log.Print(url)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		req.Header.Add("Authorization", "Bearer "+token.Token)
		//req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v2+json")
		resp, err := httpClient.Do(req)
		if err != nil {
			return err
		}

		// Verify if the response was ok
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("Server return non-200 status: %v", resp.Status)
		}

		log.Printf("Layer Size in Bytes: %s", resp.Header["Content-Length"])
		// //block forever at the next line
		defer resp.Body.Close()
		_, _ = ioutil.ReadAll(resp.Body)
	}
	return nil
}

func pull(image string) {
	defer timeTrack(time.Now(), "pull for "+image)
	t := strings.Split(image, ":")
	if len(t) != 2 {
		fmt.Println("Invalid argument. Expected image:tag.")
		os.Exit(1)
	}
	repository := t[0]
	tag := t[1]

	log.Printf("Pulling: %s", image)

	token, _ := getToken(repository)

	err := getManifestV1Noop(repository, tag, token)
	if err != nil {
		log.Printf("Unable to retrieve manifest v2: %s", err)
	}

	manifest, err := getManifestV2(repository, tag, token)
	if err != nil {
		log.Printf("Unable to retrieve manifest v2: %s", err)
	}

	err = getLayers(repository, manifest, token)
	if err != nil {
		log.Printf("Unable to retrieve manifest: %s", err)
	}

}

func init() {
	httpClient.Concurrency = 10
	httpClient.MaxRetries = 5
	httpClient.Backoff = pester.ExponentialBackoff
}

func main() {
	var fileStr string
	flag.StringVar(&fileStr, "file", "", "file with a list of repositories to pull")
	flag.Parse()

	if fileStr == "" {
		log.Fatal("File is required")
	}

	file, err := os.Open(fileStr)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 0 {
			pull(line)
		}
	}

	if err = scanner.Err(); err != nil {
		log.Fatal(err)
	}
}
