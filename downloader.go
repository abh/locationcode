package main

import (
	"errors"
	"fmt"
	"path"
	"time"

	alphafoxtrot "github.com/grumpypixel/go-airport-finder"
	"github.com/grumpypixel/go-webget"
)

// Download csv files from OurAirports.com
func DownloadDatabase(targetDir string) error {
	files := make([]string, 0)
	for _, filename := range alphafoxtrot.OurAirportsFiles {
		files = append(files, alphafoxtrot.OurAirportsBaseURL+filename)
	}
	var errs []error
	for _, url := range files {
		options := webget.Options{
			ProgressHandler: MyProgress{},
			Timeout:         time.Second * 60,
			CreateTargetDir: true,
		}
		err := webget.DownloadToFile(url, targetDir, "", &options)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type MyProgress struct{}

func (p MyProgress) Start(sourceURL string) {
	// fmt.Println()
}

func (p MyProgress) Update(sourceURL string, percentage float64, bytesRead, contentLength int64) {
	if percentage > 0 {
		fmt.Printf("\rDownloading %s: %v bytes [%.2f%%]", path.Base(sourceURL), bytesRead, percentage)
	}
	fmt.Printf("\rDownloading %s: %v bytes [done]", path.Base(sourceURL), bytesRead)
}

func (p MyProgress) Done(sourceURL string) {
	fmt.Println()
}
