package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
	"io"
	"io/ioutil"
	"mime/multipart"
	"bytes"
	"gopkg.in/yaml.v2"
)

var clientID string
var clientSecret string
var projectOptionsId string

var httpClient = &http.Client{Timeout: 200 * time.Second}

func sdlLoginURL() string {
	return "https://languagecloud.sdl.com/tm4lc/api/v1/auth/token"
}

func sdlProjectOptionsURL() string{
	return "https://languagecloud.sdl.com/tm4lc/api/v1/projects/options"
}

func sdlLanguagesURL() string{
	return "https://languagecloud.sdl.com/tm4lc/api/v1/languages/list"
}

func sdlUploadUrl(projectOptionId string) string{
	return "https://languagecloud.sdl.com/tm4lc/api/v1/files/"+projectOptionId
}

func sdlCreateProjectURL() string{
	return "https://languagecloud.sdl.com/tm4lc/api/v1/projects"
}

func sdlPortalProjectDetailsURL(projectId string) string {
	return "https://languagecloud.sdl.com/en/managed-translation/detail?jobId="+projectId
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}

type SDLJobTemplateConfiguration struct {
	Name             string                               `yaml:"name"`
	Source           string                               `yaml:"source"`
	ProjectOption 	 string 							  `yaml:"project_option"`
	Source_language  string                               `yaml:"source_language"`
	Target_languages []string                             `yaml:"target_languages"`
}

type SDLConfiguration struct {
	Job_template SDLJobTemplateConfiguration `yaml:"job_template"`
}

func (config *SDLConfiguration) readFromFile(filepath string) *SDLConfiguration {
	yamlFile, err := ioutil.ReadFile(filepath)
	if err != nil {
		log.Fatal(err)
		return nil
	}
	err = yaml.Unmarshal(yamlFile, config)
	if err != nil {
		log.Fatal(err)
		return nil
	}

	return config
}

type AuthenticateResponse struct {
	Access_token string `json:"access_token"`
	Expires_in   int    `json:"expires_in"`
	Token_type   string `json:"token_type"`
	Refresh_token string `json:"refresh_token"`

}

func authenticate(clientID string, clientSecret string, userName string,password string,target interface{})error{
	var bodyString = "grant_type=password"
	bodyString += "&client_id=" + clientID
	bodyString += "&client_secret=" + clientSecret
	bodyString += "&username=" + userName
	bodyString += "&password=" + password

	body := strings.NewReader(bodyString)
	req, err := http.NewRequest("POST", sdlLoginURL(), body)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	return json.NewDecoder(resp.Body).Decode(target)
}

type ProjectOption struct{
	ProjectOptionsId string `json:"Id"`
	ProjectOptionName string `json:"Name"`
}


func getProjectOptions(auth AuthenticateResponse,target interface{})error{
	var bodyString = ""

	body := strings.NewReader(bodyString)

	req,err := http.NewRequest("GET",sdlProjectOptionsURL(),body)

	authorization_value := auth.Token_type + " " + auth.Access_token
	req.Header.Set("Authorization", authorization_value)

	resp, err := httpClient.Do(req)
	if err!=nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	// responseData, sErr := ioutil.ReadAll(resp.Body)
	// if sErr != nil {

	// }
	// fmt.Println(string(responseData))

	return json.NewDecoder(resp.Body).Decode(target)
}

type Attachment struct {
	AttachmentFilePath string
}

// https://stackoverflow.com/questions/20205796/post-data-using-the-content-type-multipart-form-data
func mustOpen(filePath string) *os.File {
	fileReader, err := os.Open(filePath)
	if err != nil {
		log.Fatal(err)
	}
	return fileReader
}

type UploadResponse struct{
	FileId string `json:"FileId"`
	FileName string `json:"FileName"`
}

func upload(client *http.Client, url string, auth AuthenticateResponse, values map[string]io.Reader,target interface{}) (err error) {
	// Prepare a form that you will submit to that URL.
	print("Uploading...\n")
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for key, r := range values {
		var fw io.Writer
		if x, ok := r.(io.Closer); ok {
			defer x.Close()
		}
		// Add an image file
		if x, ok := r.(*os.File); ok {
			if fw, err = w.CreateFormFile(key, x.Name()); err != nil {
				return
			}
		} else {
			// Add other fields
			if fw, err = w.CreateFormField(key); err != nil {
				return
			}
		}
		if _, err = io.Copy(fw, r); err != nil {
			return err
		}

	}
	// Don't forget to close the multipart writer.
	// If you don't close it, your request will be missing the terminating boundary.
	w.Close()

	// Now that you have a form, you can submit it to your handler.
	req, err := http.NewRequest("POST", url, &b)
	if err != nil {
		return
	}
	// Don't forget to set the content type, this will contain the boundary.
	req.Header.Set("Content-Type", w.FormDataContentType())

	authorization_value := auth.Token_type + " " + auth.Access_token
	req.Header.Set("Authorization", authorization_value)

	// Submit the request
	res, err := client.Do(req)
	if err != nil {
		return
	}

	defer res.Body.Close()

	// Check the response
	if res.StatusCode != http.StatusCreated {
		err = fmt.Errorf("bad status: %s", res.Status)
	}

	return json.NewDecoder(res.Body).Decode(target)
}

func uploadAttachment(attachment Attachment, auth AuthenticateResponse, projectOptionId string,target interface{}){
	jsonData := new(bytes.Buffer)
	json.NewEncoder(jsonData).Encode(attachment)

	values := map[string]io.Reader{
		"file": mustOpen(attachment.AttachmentFilePath),
		"json": jsonData,
	}
	err := upload(httpClient, sdlUploadUrl(projectOptionId), auth, values,&target)
	if err != nil {
		log.Fatal(err)
	}

}

type Language struct{
	CultureCode string `json:"CultureCode"`
}

func getAllLanguages(auth AuthenticateResponse,target interface{})error{
	var bodyString = ""

	body := strings.NewReader(bodyString)

	req,err := http.NewRequest("GET",sdlLanguagesURL(),body)

	authorization_value := auth.Token_type + " " + auth.Access_token
	req.Header.Set("Authorization", authorization_value)

	resp, err := httpClient.Do(req)
	if err!=nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	responseData, sErr := ioutil.ReadAll(resp.Body)
	if sErr != nil {
		log.Fatal(sErr)
	}
	fmt.Println("All languages are "+string(responseData))

	return json.NewDecoder(resp.Body).Decode(target)
}

type Project struct{
	Name string
	ProjectOptionsId string
	SrcLang string
	Files   []File
}

type File struct{
	FileID  string   `json:"fileId"`
	Targets []string `json:"targets"`
}

type ProjectResponse struct{
	Result int `json:"Result"`
	ProjectId string `json:"ProjectId"`
}

func createProject(auth AuthenticateResponse,project Project,target interface{})error{
	body := new(bytes.Buffer)
	json.NewEncoder(body).Encode(project)

	fmt.Println("Create project body:"+body.String())

	req, err := http.NewRequest("POST", sdlCreateProjectURL(), body)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	authorization_value := auth.Token_type + " " + auth.Access_token
	req.Header.Set("Authorization", authorization_value)

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Println("Created project")
	} else {
		fmt.Println("Failed to create project")
		//fmt.Println(resp)
		responseData, sErr := ioutil.ReadAll(resp.Body)
		if sErr != nil {
		log.Fatal(sErr)
		}
		fmt.Println(string(responseData))
		os.Exit(1)
	}

	/*responseData, sErr := ioutil.ReadAll(resp.Body)
	if sErr != nil {
		log.Fatal(sErr)
	}
	fmt.Println(string(responseData))*/

	return json.NewDecoder(resp.Body).Decode(target)
}

func main() {

	sdlConfigFilepath := getenv("sdl_config", "sdl.yml")

	var configuration SDLConfiguration
	configuration.readFromFile(sdlConfigFilepath)


	clientID := getenv("sdl_client_id","")
	clientSecret := getenv("sdl_client_secret","")
	userName := getenv("sdl_username","")
	password := getenv("sdl_password","") 

	if clientID == "" {
		fmt.Println("Client ID is required\n")
		os.Exit(1)
	}

	if clientSecret == "" {
		fmt.Println("Client secret is required\n")
		os.Exit(1)
	}

	if userName == "" {
		fmt.Println("Username is required\n")
		os.Exit(1)
	}

	if password == "" {
		fmt.Println("Password is required\n")
		os.Exit(1)
	}

	auth := AuthenticateResponse{}
	authenticate(clientID, clientSecret, userName, password,&auth)
	if auth.Access_token == "" {
		fmt.Println("Failed to authenticate with SDL")
		os.Exit(1)
	}

	fmt.Printf("Access_token is %s- Token_type is %s",auth.Access_token,auth.Token_type)


	var projectOptions []ProjectOption

	getProjectOptions(auth,&projectOptions)

	for _, value := range projectOptions {

		if strings.EqualFold(value.ProjectOptionName,configuration.Job_template.ProjectOption) {
			projectOptionsId = value.ProjectOptionsId
			fmt.Println(value.ProjectOptionsId)
			break
		}
	}

	

	attachment := Attachment{}
	attachment.AttachmentFilePath = configuration.Job_template.Source

	var uploadResponse []UploadResponse
	uploadAttachment(attachment, auth,projectOptionsId,&uploadResponse)

	fmt.Println("Uploaded file id is "+uploadResponse[0].FileId)

	currentTime := time.Now()
	dateString := currentTime.Format("20060102")

	project:= Project{}
	project.ProjectOptionsId = projectOptionsId
	project.Name = configuration.Job_template.Name + "-" + dateString
	project.SrcLang = configuration.Job_template.Source_language

	file := File{}
	file.FileID = uploadResponse[0].FileId
	file.Targets = configuration.Job_template.Target_languages

	files := make([]File, 0)
	files = append(files, file)

	project.Files = files



	projectResponse:= ProjectResponse{}
	createProject(auth,project,&projectResponse)

	fmt.Println("Project id is "+projectResponse.ProjectId)
	projectUrl:= sdlPortalProjectDetailsURL(projectResponse.ProjectId)

	fmt.Println(projectUrl)


	//
	// --- Step Outputs: Export Environment Variables for other Steps:
	// You can export Environment Variables for other Steps with
	//  envman, which is automatically installed by `bitrise setup`.
	// A very simple example:
	cmdLog, err := exec.Command("bitrise", "envman", "add", "--key", "SDL_PROJECT_DETAIL_URL", "--value", projectUrl).CombinedOutput()
	if err != nil {
		fmt.Printf("Failed to expose output with envman, error: %#v | output: %s", err, cmdLog)
		os.Exit(1)
	}
	//You can find more usage examples on envman's GitHub page
	//at: https://github.com/bitrise-io/envman

	//
	// --- Exit codes:
	// The exit code of your Step is very important. If you return
	//  with a 0 exit code `bitrise` will register your Step as "successful".
	// Any non zero exit code will be registered as "failed" by `bitrise`.
	os.Exit(0)
}
