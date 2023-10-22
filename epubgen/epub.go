package epubgen

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/bmaupin/go-epub"
	"github.com/go-shiori/go-readability"
	"github.com/gosimple/slug"
	"github.com/nikhil1raghav/kindle-send/config"
	"github.com/nikhil1raghav/kindle-send/util"
)

type epubmaker struct {
	Epub      *epub.Epub
	downloads sync.Map
}

func NewEpubmaker(title string) *epubmaker {
	downloadMap := sync.Map{}
	return &epubmaker{
		Epub:      epub.NewEpub(title),
		downloads: downloadMap,
	}
}

func fetchReadable(url string) (readability.Article, error) {
	var article readability.Article
	var err error
	for retry := 1; retry <= 3; retry++ {
		article, err = readability.FromURL(url, 30*time.Second)
		if err == nil && !strings.Contains(article.Title, "502") {
			break
		}
		fmt.Printf("Failed to fetch readable content from %s: %s  will retry %d \n", url, article.Title, retry)
		time.Sleep(3 * time.Second)
	}

	if err != nil {
		return readability.Article{}, err
	}

	return article, nil
}

// Point remote image link to downloaded image
func (e *epubmaker) changeRefs(i int, img *goquery.Selection) {
	img.RemoveAttr("loading")
	img.RemoveAttr("srcset")
	imgSrc, exists := img.Attr("src")
	if exists {

		src, o := e.downloads.Load(imgSrc)
		if _, ok := src, o; ok {
			util.Green.Printf("Setting img src from %s to %s \n", imgSrc, src)
			img.SetAttr("src", src.(string))
		}
	}
}

func (e *epubmaker) downloadImages(i int, img *goquery.Selection, tmpFolder string) {
	util.CyanBold.Println("Downloading Images")
	imgSrc, exists := img.Attr("src")

	if exists {
		if _, ok := e.downloads.Load(imgSrc); ok {
			return
		}
		var imgRef string
		var err error
		var imgPath string

		for retry := 1; retry <= 3; retry++ {
			imageFileName := util.GetHash(imgSrc)
			imgPath, err = downloadImage(imgSrc, imageFileName, tmpFolder)

			if err != nil {
				util.Red.Printf("Couldn't download image %s: %s\n will retry in 3 seconds\n", imgSrc, err)
				time.Sleep(3 * time.Second)
				continue
			}

			imgRef, err = e.Epub.AddImage(imgPath, imageFileName)
			if err == nil {
				break
			}
		}
		if err != nil {
			util.Red.Printf("------ Couldn't add image %s : %s\n", imgSrc, err)
			return
		} else {
			util.Green.Printf("Downloaded image %s\n", imgSrc)
			e.downloads.Store(imgSrc, imgRef)
		}
	}
}
func downloadImage(url, filename string, tmpFolder string) (string, error) {
	filePath := filepath.Join(tmpFolder, filename)
	if _, err := os.Stat(filePath); err == nil {
		util.Green.Printf("Skip download, %s already existed %s \n", url, filePath)
		return filePath, nil
	}
	var response *http.Response
	var err error
	var attempts int
	const maxAttempts = 3
	for {
		attempts++
		response, err = http.Get(url)
		if err != nil || response.StatusCode != 200 {
			if attempts < maxAttempts {
				util.Red.Printf("Failed to get %s at %d try \n", url, attempts+1)
				time.Sleep(time.Second)
				continue
			}
			util.Red.Printf("Can not to get %s after 3 attemps \n", url)
			return "", err
		}
		break
	}
	defer response.Body.Close()
	file, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = io.Copy(file, response.Body)
	if err != nil {
		_ = os.Remove(filePath)
		return "", err
	}
	return filePath, nil
}

// Fetches images in article and then embeds them into epub
func (e *epubmaker) embedImages(wg *sync.WaitGroup, article *readability.Article, tmpFolder string) {
	util.Cyan.Println("Embedding images in ", article.Title)
	defer wg.Done()
	//TODO: Compress images before embedding to improve size
	doc := goquery.NewDocumentFromNode(article.Node)

	//download all images
	doc.Find("img").Each(func(i int, img *goquery.Selection) {
		e.downloadImages(i, img, tmpFolder)
	})

	//Change all refs, doing it in two phases to download repeated images only once
	doc.Find("img").Each(e.changeRefs)

	content, err := doc.Html()

	if err != nil {
		util.Red.Printf("Error converting modified %s to HTML, it will be transferred without images : %s \n", article.Title, err)
	} else {
		article.Content = content
	}
}

// TODO: Look for better formatting, this is bare bones
func prepare(article *readability.Article) string {
	return "<h1>" + article.Title + "</h1>" + article.Content
}

// Add articles to epub
func (e *epubmaker) addContent(articles *[]readability.Article) error {
	added := 0
	for _, article := range *articles {
		_, err := e.Epub.AddSection(prepare(&article), article.Title, "", "")
		if err != nil {
			util.Red.Printf("Couldn't add %s to epub : %s", article.Title, err)
		} else {
			added++
		}
	}
	util.Green.Printf("Added %d articles\n", added)
	if added == 0 {
		return errors.New("No article was added, epub creation failed")
	}
	return nil
}

// Generates a single epub from a slice of urls, returns file path
func Make(pageUrls []string, title string, coverUrl string) (string, error) {
	tmpFolder := fmt.Sprintf("tmp-%s", util.GenHash(strings.Join(pageUrls, "-")))
	_ = os.Mkdir(tmpFolder, 0755)
	defer os.RemoveAll(tmpFolder)

	readableArticles := getReadableArticles(pageUrls)
	if len(readableArticles) == 0 {
		return "", errors.New("No readable url given, exiting without creating epub")
	}

	if len(title) == 0 {
		title = readableArticles[0].Title
		util.Magenta.Printf("No title supplied, inheriting title of first readable article : %s \n", title)
	}

	book := NewEpubmaker(title)

	//get images and embed them
	var wg sync.WaitGroup

	for i := 0; i < len(readableArticles); i++ {
		wg.Add(1)
		go book.embedImages(&wg, &readableArticles[i], tmpFolder)
	}

	wg.Wait()

	err := book.addContent(&readableArticles)
	if len(coverUrl) != 0 {
		coverImagePath, _ := book.Epub.AddImage(coverUrl, "cover.png")
		book.Epub.SetCover(coverImagePath, "")
	}

	if err != nil {
		return "", err
	}
	storeDir, err := getStoreDir(err)

	titleSlug := slug.Make(title)
	var filename string
	if len(titleSlug) == 0 {
		filename = "kindle-send-doc-" + tmpFolder + ".epub"
	} else {
		filename = titleSlug + ".epub"
	}
	filepath := path.Join(storeDir, filename)
	err = book.Epub.Write(filepath)
	if err != nil {
		return "", err
	}

	return filepath, nil
}

type ReadableResult struct {
	Index   int
	Article readability.Article
}

func fetchURL(url string, index int, ch chan<- ReadableResult) {
	article, err := fetchReadable(url)
	if err != nil {
		util.Red.Printf("Couldn't convert %s because %s", url, err)
		util.Magenta.Println("SKIPPING ", url)
	}
	util.Green.Printf("Fetched %s --> %s\n", url, article.Title)
	ch <- ReadableResult{Article: article, Index: index}
}
func getReadableArticles(pageUrls []string) []readability.Article {
	readableArticles := make([]readability.Article, len(pageUrls))
	ch := make(chan ReadableResult, len(pageUrls))
	for i, url := range pageUrls {
		go fetchURL(url, i, ch)
	}
	for i := 0; i < len(pageUrls); i++ {
		article := <-ch
		readableArticles[article.Index] = article.Article
	}
	return readableArticles
}

func getStoreDir(err error) (string, error) {
	var storeDir string
	if len(config.GetInstance().StorePath) == 0 {
		storeDir, err = os.Getwd()
		if err != nil {
			util.Red.Println("Error getting current directory, trying fallback")
			storeDir = "./"
		}
	} else {
		storeDir = config.GetInstance().StorePath
	}
	return storeDir, err
}
