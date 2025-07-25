package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func main() {

	localList := openLocal()

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
		if v.PageCount >= 2 {
			startTime := fileLegth.Media.Chapters[len(fileLegth.Media.Chapters)-(v.PageCount+1)].End

			fmt.Println("new chapter: " + v.Name)

			for _, j := range users {

				tjekBookForUpdate(localList.URL, j.Token, j.Username, v.ID, v.Name, startTime)

			}
		}
	}
	localList.Books = newBookList
	saveLocal(localList)
}

func updateChapters(url string, api string, itemId string, bookName string) {
	fmt.Println("adding new pagers to: " + bookName)
	data, _, err := getAPI(url+"/api/items/"+itemId, api)
	if err != nil {
		fmt.Println("chapter update faild")
		return
	}

	var apiData bookInfo
	json.Unmarshal(data, &apiData)

	var chapterList bookChapters
	var addetTime float64 = 0

	for i, v := range apiData.Media.AudioFiles {
		newTime := addetTime + v.Duration
		chapterList.Chapters = append(chapterList.Chapters, bookChapter{ID: i, Start: addetTime, End: newTime, Title: string(v.Metadata.FileName[0 : len(v.Metadata.FileName)-4])})
		addetTime = newTime
	}

	jsonData, _ := json.Marshal(chapterList)

	client := &http.Client{}
	req, _ := http.NewRequest("POST", url+"/api/items/"+itemId+"/chapters", bytes.NewReader(jsonData))

	req.Header.Add("Authorization", " Bearer "+api)
	req.Header.Add("Content-Type", "application/json")

	client.Do(req)
}

func tjekBookForUpdate(URL string, userKEY string, userName string, bookID string, bookName string, tagetTime float64) {

	pro := getMediaProgress(URL, userKEY, bookID)
	if !pro.IsFinished {
		return
	}
	courentPos := pro.CurrentTime

	fmt.Println("updater (" + bookName + ") for user (" + userName + ")")

	if pro.Duration == tagetTime {
		fmt.Println("book status is not update <<30 sec for: " + userName)
		updateMediaProgress(URL, userKEY, bookID, jsonPrograsSetter{CurrentTime: tagetTime - 30})
	} else {
		fmt.Println("setter timmer for new page for: " + userName)
		updateMediaProgress(URL, userKEY, bookID, jsonPrograsSetter{CurrentTime: tagetTime})
	}
	pro = getMediaProgress(URL, userKEY, bookID)
	if pro.IsFinished {
		fmt.Println("update faild trying to bypass")
		updateMediaProgress(URL, userKEY, bookID, jsonPrograsSetter{CurrentTime: courentPos - 30})
	}

}

func openLocal() (Data localData) {
	fmt.Println("load data...")
	dataByte, err := os.ReadFile("./info.json")
	if errors.Is(err, os.ErrNotExist) {
		saveLocal(Data)
	}
	json.Unmarshal(dataByte, &Data)
	return
}

func saveLocal(data localData) {
	fmt.Println("save data...")
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
			//sendAltert(gotifyUrl, gotifyApi, "need update", v.Media.Metadata.Title+" file count dont match chapters")
			updateChapters(url, api, v.ID, v.Media.Metadata.Title)
		}
		pageList = append(pageList, simpelBook{ID: v.ID, PageCount: v.Media.NumChapters, Name: v.Media.Metadata.Title})
	}
	return
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

func updateMediaProgress(url string, apiKey string, bookID string, data any) {
	jsonData, _ := json.Marshal(data)

	client := &http.Client{}
	req, _ := http.NewRequest("PATCH", url+"/api/me/progress/"+bookID, bytes.NewReader(jsonData))

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

	Books []simpelBook
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
	StartedAt     int     `json:"startedAt"`
	Duration      float64 `json:"duration"`
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

type bookInfo struct {
	Media struct {
		AudioFiles []struct {
			ID       int     `json:"index"`
			Duration float64 `json:"duration"`

			Metadata struct {
				FileName string `json:"filename"`
			} `json:"metadata"`
		} `json:"audioFiles"`
	} `json:"media"`
}

type bookChapter struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Title string  `json:"title"`
}
type bookChapters struct {
	Chapters []bookChapter `json:"chapters"`
}
