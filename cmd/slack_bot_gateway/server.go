package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"

	"github.com/mattn/go-jsonpointer"
)

type (
	// Config describes server config.
	Config struct {
		LogDirPath string          `json:"log_dir"`
		ServerHost string          `json:"host"`
		Backends   []BackendConfig `json:"backends"`
	}

	// BackendConfig describes backend server host.
	BackendConfig struct {
		CallbackPrefix string `json:"callback_prefix"`
		Host           string `json:"host"`
	}
)

func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}

func readConfig(configsDirPath string) (Config, error) {
	config := Config{}

	configFilePath := filepath.Join(configsDirPath, "config.json")
	if !fileExists(configFilePath) {
		return config, fmt.Errorf("Config file does not exist: %s", configFilePath)
	}

	jsonContent, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		return config, err
	}

	if err := json.Unmarshal(jsonContent, &config); err != nil {
		return config, err
	}

	return config, nil
}

func getCallbackIDFromPayload(payloadJSON string) (string, error) {
	var payload interface{}
	err := json.Unmarshal([]byte(payloadJSON), &payload)
	if err != nil {
		return "", err
	}

	//params, _ := json.Marshal(payload)
	//fmt.Println(string(prettyParams))

	iRequestType, err := jsonpointer.Get(payload, "/type")
	if err != nil {
		return "", err
	}
	requestType := iRequestType.(string)

	var iCallbackID interface{}
	switch requestType {
	case "shortcut":
		iCallbackID, _ = jsonpointer.Get(payload, "/callback_id")
	case "view_submission":
		iCallbackID, _ = jsonpointer.Get(payload, "/view/callback_id")
	case "block_actions":
		iCallbackID, _ = jsonpointer.Get(payload, "/actions/0/action_id")
	}

	return iCallbackID.(string), nil
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: slack_bot_gateway [flags]\n")
		flag.PrintDefaults()
	}

	pConfigsDirPath := flag.String("c", "", "Configs dir path")
	flag.Parse()

	if !fileExists(*pConfigsDirPath) {
		fmt.Println("Config dir path does not exist")
		return
	}

	configsDirPath := *pConfigsDirPath

	var err error
	serverConfig, err := readConfig(configsDirPath)
	if err != nil {
		panic(err)
	}

	appLogFile, err := os.OpenFile(filepath.Join(serverConfig.LogDirPath, "app.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		panic(err)
	}
	logger := log.New(appLogFile, "logger: ", log.Ldate|log.Ltime|log.Lshortfile)

	// reverse proxy setting
	director := func(request *http.Request) {
		requestURL := *request.URL

		buffer, err := ioutil.ReadAll(request.Body)
		if err != nil {
			log.Fatal(err.Error())
		}

		params, err := url.ParseQuery(string(buffer))
		//logger.Println(params)

		payloadJSON := params.Get("payload")
		logger.Println(payloadJSON)

		callbackID, err := getCallbackIDFromPayload(payloadJSON)
		if err != nil {
			logger.Println("Failed fetching callbackID.")
			logger.Println(err)
		}

		requestURL.Scheme = "http"

		if len(callbackID) > 0 {

			re := regexp.MustCompile(`__.*`)
			callbackPrefix := re.ReplaceAllString(callbackID, "")

			for _, backendConfig := range serverConfig.Backends {
				if backendConfig.CallbackPrefix == callbackPrefix {
					requestURL.Host = backendConfig.Host
					break
				}
			}
		}

		req, err := http.NewRequest(request.Method, requestURL.String(), bytes.NewBuffer(buffer))
		if err != nil {
			log.Fatal(err.Error())
		}
		req.Header = request.Header
		*request = *req
	}

	rp := &httputil.ReverseProxy{
		Director: director,
	}

	server := http.Server{
		Addr:    serverConfig.ServerHost,
		Handler: rp,
	}

	logger.Printf("Starting gateway server on host %s...\n", serverConfig.ServerHost)

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err.Error())
	}
}
