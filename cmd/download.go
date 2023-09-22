package cmd

import (
	"os"

	"github.com/lithammer/dedent"
	"github.com/nikhil1raghav/kindle-send/classifier"
	"github.com/nikhil1raghav/kindle-send/config"
	"github.com/nikhil1raghav/kindle-send/handler"
	"github.com/nikhil1raghav/kindle-send/util"
	"github.com/spf13/cobra"
)

func init() {
	flags := downloadCmd.Flags()
	flags.StringVarP(&outputFileName, "output", "o", "", "Output file name")
	flags.StringVarP(&coverUrl, "cover-image", "i", "", "cover image url ")
	rootCmd.AddCommand(downloadCmd)
}

var (
	outputFileName string
	coverUrl       string

	helpDownload = `Downloads the webpage or collection of webpages from given arguments
that can be a standalone link or a text file containing multiple links.
Supports multiple arguments. Each argument is downloaded as a separate file.`

	exampleDownload = dedent.Dedent(`
		# Download a single webpage
		kindle-send download -o output-file-name -i cover.jpg "http://paulgraham.com/alien.html"

		# Download multiple webpages
		kindle-send download -o output-file-name -i cover.jpg "http://paulgraham.com/alien.html" "http://paulgraham.com/hwh.html"

		# Download webpage and collection of webpages
		kindle-send download -o output-file-name -i cover.jpg "http://paulgraham.com/alien.html" links.txt`,
	)
)

var downloadCmd = &cobra.Command{
	Use:     "download [LINK1] [LINK2] [FILE1] [FILE2]",
	Short:   "Download the webpage as ebook and save locally",
	Long:    helpDownload,
	Example: exampleDownload,
	Run: func(cmd *cobra.Command, args []string) {
		configPath, _ := cmd.Flags().GetString("config")
		_, err := config.Load(configPath)
		if err != nil {
			util.Red.Println(err)
			return
		}

		downloadRequests := classifier.Classify(args)
		downloadedRequests := handler.Queue(downloadRequests, outputFileName, coverUrl)

		util.CyanBold.Printf("Downloaded %d files :\n", len(downloadRequests))
		for idx, req := range downloadedRequests {
			fileInfo, _ := os.Stat(req.Path)
			util.Cyan.Printf("%d. %s\n", idx+1, fileInfo.Name())
		}
	},
}
