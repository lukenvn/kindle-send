package handler

import (
	"github.com/nikhil1raghav/kindle-send/config"
	"github.com/nikhil1raghav/kindle-send/epubgen"
	"github.com/nikhil1raghav/kindle-send/mail"
	"github.com/nikhil1raghav/kindle-send/types"
	"github.com/nikhil1raghav/kindle-send/util"
)

func Queue(downloadRequests []types.Request, outputFileName string, coverUrl string) []types.Request {
	var processedRequests []types.Request
	for _, req := range downloadRequests {
		switch req.Type {
		case types.TypeFile:
			processedRequests = append(processedRequests, req)
			continue
		case types.TypeUrl:
			path, err := epubgen.Make([]string{req.Path}, outputFileName, coverUrl)
			if err != nil {
				util.Red.Printf("SKIPPING %s : %s\n", req.Path, err)
			} else {
				processedRequests = append(processedRequests, types.NewRequest(path, types.TypeFile, nil))
			}
		case types.TypeUrlFile:
			links := util.ExtractLinks(req.Path)
			path, err := epubgen.Make(links, outputFileName, coverUrl)
			if err != nil {
				util.Red.Printf("SKIPPING %s : %s\n", req.Path, err)
			} else {
				processedRequests = append(processedRequests, types.NewRequest(path, types.TypeFile, nil))
			}
		}
	}
	return processedRequests
}

func Mail(mailRequests []types.Request, timeout int) {
	var filePaths []string
	for _, req := range mailRequests {
		filePaths = append(filePaths, req.Path)
	}
	if timeout < 60 {
		timeout = config.DefaultTimeout
	}
	mail.Send(filePaths, timeout)
}
