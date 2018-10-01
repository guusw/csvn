package main

import (
	"encoding/xml"
	"os/exec"
)

type svnInfo struct {
	Entries []svnInfoEntry `xml:"entry"`
}

type svnInfoRepository struct {
	Root string `xml:"root"`
	UUID string `xml:"uuid"`
}

type svnInfoWorkingCopy struct {
	AbsoluteRootPath string `xml:"wcroot-abspath"`
	Schedule         string `xml:"schedule"`
	Depth            string `xml:"depth"`
}

type svnInfoEntry struct {
	Kind        string `xml:"kind,attr"`
	Path        string `xml:"path,attr"`
	Revision    string `xml:"revision,attr"`
	URL         string `xml:"url"`
	RelativeURL string `xml:"relative-url"`
	Repository  svnInfoRepository  `xml:"repository"`
	WorkingCopy svnInfoWorkingCopy `xml:"wc-info"`
}

func GetSVNInfo(path string) (info svnInfo, err error) {
	svnCmd := exec.Command(svnPath, "info", "--xml", path)
	outputBytes, err := svnCmd.Output()
	if err != nil {
		return
	}

	err = xml.Unmarshal(outputBytes, &info)
	if err != nil {
		return
	}
	return
}
