package main

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"git.bakje.coffee/guus/csvn/aurora"

	"golang.org/x/crypto/ssh/terminal"
)

type svnLog struct {
	LogEntries []svnLogEntry `xml:"logentry"`
}

type svnLogPath struct {
	Kind     string `xml:"kind,attr"`
	Action   string `xml:"action,attr"`
	PropMods bool   `xml:"prop-mods,attr"`
	TextMods bool   `xml:"text-mods,attr"`
	Path     string `xml:",chardata"`
}

type svnLogEntry struct {
	Revision int          `xml:"revision,attr"`
	Author   string       `xml:"author"`
	Date     string       `xml:"date"`
	Paths    []svnLogPath `xml:"paths>path"`
	Message  string       `xml:"msg"`
}

func svnRunDefault(cmd *exec.Cmd) {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		log.Fatalln(err)
	}
}

func main() {
	svnArgs := os.Args[1:]
	command := ""
	if len(svnArgs) > 0 {
		command = svnArgs[0]
	}

	svnCmd := exec.Command(svnPath, svnArgs...)

	// if terminal.IsTerminal(int(os.Stdout.Fd())) {
	if true {
		tWidth, _, err := terminal.GetSize(int(os.Stdout.Fd()))
		if err != nil {
			tWidth = 75
			tWidthEnv, hasTWidthEnv := os.LookupEnv("CSVN_WIDTH")
			if hasTWidthEnv {
				tWidthEnvParsed, err := strconv.ParseInt(tWidthEnv, 10, 32)
				if err == nil {
					tWidth = int(tWidthEnvParsed)
				}
			}
		}

		separatorString := strings.Repeat("=", tWidth)

		if command == "log" {
			relative := false
			// Process custom arguments
			svnArgsNew := make([]string,1,len(svnArgs)) 
			svnArgsNew[0] = svnCmd.Args[0]
			for _, arg := range svnArgs {
				if arg == "--rel" {
					// This shows any paths in the log as relative paths and strips unrelevant paths from the log
					relative = true
				} else {
					svnArgsNew = append(svnArgsNew, arg)
				}
			}
			svnArgs = svnArgsNew
			svnCmd.Args = svnArgsNew
			
			// Get prefix path for repo
			cwd, _ := os.Getwd()
			var prefixPath string
			if relative {
				svnInfo, err := GetSVNInfo(cwd)
				if err != nil {
					log.Fatalln(err)
				}
				svnInfoEntry := svnInfo.Entries[0]
				prefixPath = filepath.Clean(strings.TrimPrefix(svnInfoEntry.RelativeURL, "^"))
			}

			svnCmd.Stderr = os.Stderr
			stdOutReader, stdOutWriter := io.Pipe()
			svnCmd.Stdout = stdOutWriter

			svnCmd.Args = append(svnCmd.Args, "--xml")
			go func() {
				err := svnCmd.Run()
				if err != nil {
					log.Fatalln(err)
				}
				stdOutWriter.Close()
			}()

			dec := xml.NewDecoder(stdOutReader)

			// Parse tokens until log entries start
			for {
				tok, err := dec.Token()
				startElem, isStartElem := tok.(xml.StartElement)
				if isStartElem && startElem.Name.Local == "log" {
					dec.Token()
					break
				}
				if err != nil {
					log.Fatalf("Failed to unmarshal xml log output: %s", err)
				}
			}

			// Parse each entry individually
			for {
				var logEntry svnLogEntry
				err = dec.DecodeElement(&logEntry, nil)
				if err != nil {
					if err == io.EOF {
						// Done
						return
					}
					log.Fatalf("Failed to unmarshal xml log output: %s", err)
				}

				logTimeString := "<invalid>"
				logTime, timeErr := time.Parse(time.RFC3339, logEntry.Date)
				if timeErr == nil {
					logTimeString = logTime.Local().Format(time.RFC822)
				}

				headerString := separatorString
				revString := aurora.Sprintf(aurora.Bold("r%d"), logEntry.Revision)
				leftHeaderString := fmt.Sprintf(" %s %s ", revString, logTimeString)
				headerOffset := 4
				rightHeaderString := aurora.Sprintf(aurora.Bold(" %s "), logEntry.Author)

				remainingAfterLeftHeader := len(separatorString) - len(leftHeaderString) - headerOffset
				remainingAfterRightHeader := headerOffset
				leftRightHeaderFill := remainingAfterLeftHeader - remainingAfterRightHeader - len(rightHeaderString)
				if remainingAfterLeftHeader < 0 {
					remainingAfterLeftHeader = 0
				}
				if remainingAfterRightHeader < 0 {
					remainingAfterRightHeader = 0
				}
				if leftRightHeaderFill < 0 {
					leftRightHeaderFill = 0
				}

				headerString = separatorString[0:headerOffset] + leftHeaderString + separatorString[:leftRightHeaderFill] + rightHeaderString + separatorString[:remainingAfterRightHeader]
				fmt.Println(headerString)

				msgIndent := "  "
				msg := msgIndent + strings.Replace(logEntry.Message, "\n", "\n"+msgIndent, -1)
				fmt.Println(msg)

				for _, entryPath := range logEntry.Paths {
					var actionColor aurora.Color
					switch entryPath.Action {
					case "A":
						actionColor = aurora.GreenFg
					case "D":
						actionColor = aurora.RedFg
					case "M":
						actionColor = aurora.BlueFg
					}

					if entryPath.Kind == "dir" {
						actionColor |= aurora.ItalicFm
					}
					
					entryFilePath := filepath.Clean(entryPath.Path)
					if relative {
						if !strings.HasPrefix(entryFilePath, prefixPath) {
							continue // Filter ignored paths
						}
						
						relativePath, err := filepath.Rel(prefixPath, entryFilePath)
						if err == nil {
							entryFilePath = relativePath
						}
					}

					pathString := aurora.Colorize(entryFilePath, actionColor)
					fmt.Printf("  %s %s\n",
						aurora.Colorize(entryPath.Action, actionColor),
						pathString)
				}
				fmt.Println() // Empty line
			}
		} else if command == "diff" {
			svnCmd.Stderr = os.Stderr
			stdOutReader, stdOutWriter := io.Pipe()
			svnCmd.Stdout = stdOutWriter

			svnCmd.Args = append(svnCmd.Args)
			go func() {
				err := svnCmd.Run()
				if err != nil {
					log.Fatalln(err)
				}
				stdOutWriter.Close()
			}()

			reader := bufio.NewReader(stdOutReader)

			for {
				lineBytes, _, err := reader.ReadLine()
				if err != nil {
					log.Fatalln(err)
				}

				line := string(lineBytes)
				if strings.HasPrefix(line, "-") {
					line = aurora.Sprintf(aurora.Red("%s"), line)
				} else if strings.HasPrefix(line, "+") {
					line = aurora.Sprintf(aurora.Green("%s"), line)
				} else if strings.HasPrefix(line, "@@ ") {
				} else if strings.HasPrefix(line, "==") ||
					strings.HasPrefix(line, "Index: ") {
					line = aurora.Sprintf(aurora.Bold("%s"), line)
				} else {
					line = aurora.Sprintf(aurora.Gray("%s"), line)
				}
				fmt.Printf("%s\n", line)
			}
		} else if command == "status" || command == "stat" {
			svnCmd.Stderr = os.Stderr
			stdOutReader, stdOutWriter := io.Pipe()
			svnCmd.Stdout = stdOutWriter

			svnCmd.Args = append(svnCmd.Args)
			go func() {
				err := svnCmd.Run()
				if err != nil {
					log.Fatalln(err)
				}
				stdOutWriter.Close()
			}()

			reader := bufio.NewReader(stdOutReader)

			for {
				lineBytes, _, err := reader.ReadLine()
				if err != nil {
					if err == io.EOF {
						return
					}
					log.Fatalln(err)
				}

				line := string(lineBytes)
				mode := ""
				if len(line) >= 8 {
					mode = strings.TrimRight(line[:8], " ")
				}
				switch mode {
				case " M":
					line = aurora.Sprintf(aurora.Bold(aurora.Blue("%s")), line)
				case "M":
					line = aurora.Sprintf(aurora.Blue("%s"), line)
				case "!":
					line = aurora.Sprintf(aurora.Inverse(aurora.Red("%s")), line)
				case "D":
					line = aurora.Sprintf(aurora.Red("%s"), line)
				case "A":
					line = aurora.Sprintf(aurora.Green("%s"), line)
				default:
					line = aurora.Sprintf(aurora.Gray("%s"), line)
				}
				fmt.Printf("%s\n", line)
			}
		} else {
			svnRunDefault(svnCmd)
		}

	} else {
		svnRunDefault(svnCmd)
	}
}
