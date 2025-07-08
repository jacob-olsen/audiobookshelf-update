package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {

	localList := openLocal()
	var failList []faildUpdate

	//test stord info
	if localList.ApiKey == "" {
		fmt.Println("api key not set")
		os.Exit(1)
	}
	if localList.URL == "" {
		fmt.Println("URL not set")
		os.Exit(1)
	}

	//test conntion
	if !ping(localList.URL, localList.ApiKey) {
		fmt.Println("cant connet to server")
		os.Exit(1)
	}

	if len(localList.Retrys) > 0 {
		fmt.Println("updates fail last run retrying(" + strconv.Itoa(len(localList.Retrys)) + ")")
		for _, v := range localList.Retrys {

			var fileLegth fileInfo

			JFileLegth, _, err := getAPI(localList.URL+"/api/items/"+v.BookID, localList.ApiKey)
			if err != nil {
				fmt.Println(err)
			}

			json.Unmarshal(JFileLegth, &fileLegth)
			startTime := fileLegth.Media.Chapters[v.TagetPage].End

			var i int = 0
			pro := getMediaProgress(localList.URL, v.ApiKey, v.BookID)
			for pro.IsFinished {
				fmt.Println("retrying book (" + v.BookName + ") for " + v.UserName)
				updateMediaProgress(localList.URL, v.ApiKey, v.BookID, startTime)
				pro = getMediaProgress(localList.URL, v.ApiKey, v.BookID)
				if i != 0 {
					fmt.Println("update failde retry " + strconv.Itoa(i))
					time.Sleep(1 * time.Second)
					if i > 3 {
						failList = append(failList, v)
						break
					}
				}
				i++
			}
		}

	}

	//scan books
	var workList []simpelBook
	newBookList := getBookList(localList.URL, localList.ApiKey, "8be27d08-3134-4802-8e5d-c13417ed9cf2", localList.GotifyUrl, localList.GotifyApi)
	for _, v := range newBookList {
		for _, j := range localList.Books {
			if v.ID == j.ID {
				if v.PageCount > j.PageCount {
					workList = append(workList, simpelBook{ID: v.ID, Name: v.Name, PageCount: v.PageCount - j.PageCount})
				}
			}
		}
	}

	if len(workList) == 0 {
		fmt.Println("no updates fund")
		localList.Books = newBookList
		saveLocal(localList)
		os.Exit(0)
	}

	//get user list
	users := getUserList(localList.URL, localList.ApiKey)
	for _, v := range workList {
		var fileLegth fileInfo

		JFileLegth, _, err := getAPI(localList.URL+"/api/items/"+v.ID, localList.ApiKey)
		if err != nil {
			fmt.Println(err)
		}

		json.Unmarshal(JFileLegth, &fileLegth)
		startTime := fileLegth.Media.Chapters[len(fileLegth.Media.Chapters)-(v.PageCount+1)].End

		fmt.Println("new chapter: " + v.Name)

		for _, j := range users {
			var onList bool = false
			for _, nop := range failList {
				if nop.ApiKey != j.Token {
					continue
				}
				if nop.BookID != v.ID {
					continue
				}
				onList = true
				break
			}
			if !onList {
				var i int = 0
				pro := getMediaProgress(localList.URL, j.Token, v.ID)
				for pro.IsFinished {
					fmt.Println("updating for " + j.Username)
					updateMediaProgress(localList.URL, j.Token, v.ID, startTime)
					pro = getMediaProgress(localList.URL, j.Token, v.ID)
					if i != 0 {
						fmt.Println("update failde retry " + strconv.Itoa(i))
						time.Sleep(1 * time.Second)
						if i > 3 {
							failList = append(failList, faildUpdate{UserName: j.Username, ApiKey: j.Token, BookName: v.Name, BookID: v.ID, TagetPage: len(fileLegth.Media.Chapters) - (v.PageCount + 1)})
							break
						}
					}
					i++
				}
			}

		}
	}
	localList.Books = newBookList
	localList.Retrys = failList
	saveLocal(localList)
}

func openLocal() (Data localData) {
	dataByte, err := os.ReadFile("./info.json")
	if errors.Is(err, os.ErrNotExist) {
		saveLocal(Data)
	}
	json.Unmarshal(dataByte, &Data)
	return
}

func saveLocal(data localData) {
	jsonData, _ := json.Marshal(data)
	os.WriteFile("./info.json", jsonData, 0666)
}

func ping(url string, api string) bool {
	//tjek server exist
	data, _, err := getAPI(url+"/ping", api)
	if err != nil {
		fmt.Println("ping faild")
		return false
	}
	return strings.Contains(string(data), "true")
}

func getBookList(url string, api string, libary string, gotifyUrl string, gotifyApi string) (pageList []simpelBook) {
	data, _, err := getAPI(url+"/api/libraries/"+libary+"/items", api)
	if err != nil {
		fmt.Println(err)
	}

	var apiData BookList
	json.Unmarshal(data, &apiData)

	for _, v := range apiData.Results {
		if v.Media.NumAudioFiles != v.Media.NumChapters {
			sendAltert(gotifyUrl, gotifyApi, "need update", v.Media.Metadata.Title+" file count dont match chapters")
		}
		pageList = append(pageList, simpelBook{ID: v.ID, PageCount: v.Media.NumChapters, Name: v.Media.Metadata.Title})
	}
	return
}

func sendAltert(gotifyUrl string, gotifyApi string, title string, text string) {
	fmt.Println(text)
	if gotifyUrl != "" && gotifyApi != "" {
		http.PostForm(gotifyUrl+"/message?token="+gotifyApi, url.Values{"message": {text}, "title": {title}})
	}
}

func getAPI(url string, apiKey string) (body []byte, statusCode int, err error) {
	client := &http.Client{}
	req, _ := http.NewRequest("GET", url, nil)

	req.Header.Add("Authorization", " Bearer "+apiKey)

	resp, err := client.Do(req)

	if err != nil {
		fmt.Println(err)
		return
	}
	defer resp.Body.Close()
	statusCode = resp.StatusCode
	body, err = io.ReadAll(resp.Body)

	return
}

func getUserList(url string, apiKey string) (userList []UserInfo) {
	users, code, err := getAPI(url+"/api/users?", apiKey) //use status code to test for admin
	if err != nil {
		fmt.Println(err)
	}
	if code != 200 {
		fmt.Println("not admin private run!!")
		var me UserInfo
		users, _, err := getAPI(url+"/api/me", apiKey)
		if err != nil {
			fmt.Println(err)
		}
		json.Unmarshal(users, &me)

		userList = append(userList, me)
		return
	}
	type jsonUserList struct {
		Users []UserInfo `json:"users"`
	}
	var JUserList jsonUserList

	json.Unmarshal(users, &JUserList)
	userList = JUserList.Users
	return
}

func getMediaProgress(url string, apiKey string, bookID string) (info MediaProgress) {
	data, _, _ := getAPI(url+"/api/me/progress/"+bookID, apiKey)
	json.Unmarshal(data, &info)
	return
}

func updateMediaProgress(url string, apiKey string, bookID string, time float64) {
	data, _ := json.Marshal(jsonPrograsSetter{CurrentTime: time})

	client := &http.Client{}
	req, _ := http.NewRequest("PATCH", url+"/api/me/progress/"+bookID, bytes.NewReader(data))

	req.Header.Add("Authorization", " Bearer "+apiKey)
	req.Header.Add("Content-Type", "application/json")

	client.Do(req)
}

type BookList struct {
	Results []struct {
		ID        string `json:"id"`
		Path      string `json:"path"`
		RelPath   string `json:"relPath"`
		MediaType string `json:"mediaType"`
		Media     struct {
			ID       string `json:"id"`
			Metadata struct {
				Title      string `json:"title"`
				AuthorName string `json:"authorName"`
			} `json:"metadata"`
			NumTracks     int `json:"numTracks"`
			NumAudioFiles int `json:"numAudioFiles"`
			NumChapters   int `json:"numChapters"`
		} `json:"media"`
	} `json:"results"`
	Total int `json:"total"`
}

type localData struct {
	URL    string
	ApiKey string

	GotifyUrl string
	GotifyApi string

	Books  []simpelBook
	Retrys []faildUpdate
}
type simpelBook struct {
	ID        string
	Name      string
	PageCount int
}

type UserInfo struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Token    string `json:"token"`
}

type MediaProgress struct {
	LibraryItemID string  `json:"libraryItemId"`
	MediaItemID   string  `json:"mediaItemId"`
	Progress      float64 `json:"progress"`
	CurrentTime   float64 `json:"currentTime"`
	IsFinished    bool    `json:"isFinished"`
}

type fileInfo struct {
	Media struct {
		Chapters []struct {
			ID    int     `json:"id"`
			End   float64 `json:"end"`
			Title string  `json:"title"`
		} `json:"chapters"`
	} `json:"media"`
}

type jsonPrograsSetter struct {
	CurrentTime float64 `json:"currentTime"`
}

type faildUpdate struct {
	UserName string
	ApiKey   string

	BookName  string
	BookID    string
	TagetPage int
}
