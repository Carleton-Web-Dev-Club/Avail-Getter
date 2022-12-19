package main

import (
	"bufio"
	"context"
	"embed"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/antchfx/htmlquery"
	"github.com/gin-gonic/gin"
)

//go:embed templates/res.gotmpl
var resTemplate string

//go:embed static
var staticFs embed.FS

type CourseDataCache struct {
	DateObtained time.Time
	ResponseTimestamp time.Time
	Terms map[string]TermData
}

type TermData struct {
	Courses map[string][]CourseData
}

type CourseData struct {
	Total int
	Free int
	Taken int
	Crn string
	Name string
}

var _cache *CourseDataCache = nil

func getCachedDataWithContext(ctx context.Context) (*CourseDataCache, error) {
	expired := false
	if _cache == nil  || time.Since(_cache.DateObtained) > 15 * time.Minute{
		fmt.Println("Renewing Data Cache")
		expired = true
	}
	
	if !expired {
		return _cache, nil
	}
	doc, err := htmlquery.LoadURL("http://at.eng.carleton.ca/SchedulerTool/pub/scheduler.php/")
	if err != nil {
		return nil, err
	}
	
	dateElm := htmlquery.FindOne(doc, "//span[@id='lastUpdated']")
	if dateElm == nil {
		return nil, errors.New("could not get last updated")
	}
	updated := strings.Split(dateElm.FirstChild.Data, ": ")[1]
	updatedTime, err := time.Parse("2006-01-02 15:04:05", strings.TrimSpace(updated))
	if err != nil {
		fmt.Println(err)
	}

	client := http.Client{}
	data := url.Values{}
    data.Set("option", "getCourseData")

	terms := []string{"202230", "202310", "202320", "202330", "202410"}
	fmt.Println("Getting data for: ", terms)
    data.Set("termsvisible", fmt.Sprintf("[%s]", strings.Join(terms, ", ")))

	r, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://at.eng.carleton.ca/SchedulerTool/pub/scheduler_server.php", strings.NewReader(data.Encode()))
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Add("User-Agent", "avail.cwdc.carleton.ca")

	resp, err := client.Do(r)
	if err != nil {
		return nil, err
	}
	fmt.Println(resp.Status, resp.Header.Get("content-length"))
	bodyScanner := bufio.NewScanner(resp.Body)
	cdc := CourseDataCache{
		Terms: map[string]TermData{},
		DateObtained: time.Now(),
		ResponseTimestamp: updatedTime,
	}

	numCourses := 0
	for bodyScanner.Scan() {
		numCourses++
		txt := strings.Split(bodyScanner.Text(), "\t")
		taken, _ := strconv.Atoi(txt[12])
		total, _ := strconv.Atoi(txt[11])
		code := strings.ToUpper(fmt.Sprintf("%s%s", txt[2], txt[3]))
		term := txt[0]
		cd := CourseData{
			Crn: txt[1],
			Taken: taken,
			Free: total - taken,
			Total: total,
			Name: fmt.Sprintf("%s %s %s",txt[2], txt[3], txt[4]),
		}
		
		if _, ok := cdc.Terms[term]; !ok {
			cdc.Terms[term] = TermData{
				Courses: map[string][]CourseData{},
			}
		}

		if d, ok := cdc.Terms[term].Courses[code]; ok {
			cdc.Terms[term].Courses[code] = append(d, cd)
		} else {
			cdc.Terms[term].Courses[code] = []CourseData{cd}
		}

	}
	_cache = &cdc
	fmt.Println("Updated cache with courses", numCourses)
	return &cdc, nil


}
func getNameForTerm(t string) string {
	num, _ := strconv.Atoi(t)
	year := int(num / 100)
	term := num % 100
	var termName = "Unknown"
	switch (term){
		case 10: termName = "Winter"; break
		case 20: termName = "Summer"; break
		case 30: termName = "Fall"; break; 
	}
	return fmt.Sprintf("%s %d", termName, year)
}

func availabilityHandler (context *gin.Context) {
	var code = "1405"
	var dpt = "COMP"
	var term = "202310"

	if v, ok := context.GetQuery("code"); ok {
		code = v
	}

	if v, ok := context.GetQuery("term"); ok {
		term = strings.ToUpper(v)
	}

	if v, ok := context.GetQuery("dpt"); ok {
		dpt = v
	}
	fmt.Printf("%s Term: %s, DPT: %s, CODE: %s\n", time.Now(), term, dpt, code)
	cdc, err := getCachedDataWithContext(context.Request.Context())
	rcd := []CourseData{}
	if err != nil {
		fmt.Println(err)
		context.Writer.Write([]byte("Error: Contact avail[at]clarkbains[dot]com"))
		context.AbortWithError(500, err)
	} else {
		if td, ok := cdc.Terms[term]; ok {
			if cd, ok1 := td.Courses[dpt+code]; ok1 {
				rcd = cd
			}
		}
	}
	t := template.New("")
	tmpl, err := t.Parse(resTemplate)
	if err != nil {
		fmt.Println(err)
	}

	err = tmpl.Execute(context.Writer, map[string]any{
		"Courses": rcd,
		"Updated": cdc.ResponseTimestamp,
		"Obtained": cdc.DateObtained,
		"Term": term,
		"Code": dpt+code,
		"HumanTerm": getNameForTerm(term),
	})

	if err != nil {
		fmt.Println(err)
	}
}

func main(){
	engine := gin.New()
	engine.StaticFS("/public", mustFS())
	engine.GET("/api/availability", availabilityHandler)
	engine.NoRoute(func(ctx *gin.Context) {
		ctx.Redirect(302, "/public/" + ctx.Request.RequestURI)
	})
	engine.Run(":8080")
}

func mustFS() http.FileSystem {
	sub, err := fs.Sub(staticFs, "static")

	if err != nil {
		panic(err)
	}

	return http.FS(sub)
}