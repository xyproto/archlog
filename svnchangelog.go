/*
 * Program for generating a ChangeLog based on svn log
 *
 * Alexander Rødseth <rodseth@gmail.com>
 *
 * 2012-01-14
 */

package main

import (
	"fmt"
	"log"
	"exec" // os/exec for newer versions of Go
	"bytes"
	"strings"
	"os"
	"xml"
	"http"
	"html"
	"io"
	"flag"
	"strconv"
)

const (
	VERSION = "0.1"
	TU_URL  = "http://www.archlinux.org/trustedusers/"
	DEV_URL = "http://www.archlinux.org/developers/"
)

var (
	nickCache map[string]string
)

// Used when parsing svn log xml
type LogEntry struct {
	Revision string `xml:"attr"`
	Author   string
	Date     string
	Msg      string
}

// Used when parsing svn log xml
type Log struct {
	XMLName  xml.Name `xml:"log"`
	LogEntry []LogEntry
}

// Use the "svn log --xml" command to fetch log entries
func getSvnLog(entries int) (Log, os.Error) {
	svnlog := Log{LogEntry: nil}
	var cmd *exec.Cmd
	if entries == -1 {
		cmd = exec.Command("/usr/bin/svn", "log", "--xml", "-r", "HEAD:0")
	} else {
		entriesText := fmt.Sprintf("%v", entries)
		cmd = exec.Command("/usr/bin/svn", "log", "--xml", "-r", "HEAD:0", "-l", entriesText)
	}
	b, err := cmd.Output()
	if err != nil {
		log.Println("Could not run svn log")
		return svnlog, err
	}
	buffer := bytes.NewBuffer(b)
	xml.Unmarshal(buffer, &svnlog)
	return svnlog, nil
}

// Make a date from the xml version of svn log somewhat prettier
func prettyDate(date string) string {
	return strings.Split(date, "T")[0]
}

// Get the contents from an URL and return a tokenizer and a ReadCloser
func getWebPageTokenizer(url string) (*html.Tokenizer, io.ReadCloser) {
	var client http.Client
	resp, err := client.Get(url)
	if err != nil {
		log.Println("Could not retrieve " + url)
		return nil, nil
	}
	tokenizer := html.NewTokenizer(resp.Body)
	return tokenizer, resp.Body
}

// Skip N tokens, if possible. Returns true if it worked out.
func Skip(tokenizer *html.Tokenizer, n int) bool {
	for counter := 0; counter < n; counter++ {
		toktype := tokenizer.Next()
		if toktype == html.ErrorToken {
			return false
		}
	}
	return true
}

func mapLetters(letter int) int {
	if ((letter >= 'A') && (letter <= 'Z')) || ((letter >= 'a') && (letter <= 'z')) {
		return letter
	}
	switch letter {
	case 'ø', 'ö':
		return 'o'
	case 'Р', 'ð':
		return 'r'
	case 'ä', 'Á', 'á':
		return 'a'
	case 'é':
		return 'e'
	default:
		return '_'
	}
	return letter
}

// Generates a nick from the name
func generateNick(name string) string {
	if strings.Index(name, " ") == -1 {
		return name
	}
	var names []string
	// If the english-friendly name is in parenthesis
	if (strings.Index(name, "(") != -1) && (strings.Index(name, ")") != -1) {
		a := strings.Index(name, "(")
		b := strings.LastIndex(name, ")")
		centerpart := name[a+1 : b]
		names = strings.SplitN(centerpart, " ", -1)
	} else {
		names = strings.SplitN(name, " ", -1)
	}
	firstname, lastname := names[0], names[len(names)-1]
	nick := strings.Replace(strings.ToLower(strings.Map(mapLetters, string(firstname[0])+lastname)), "_", "", -1)
	return nick
}

// Find the name and email based on a nick name and an URL to an
// ArchLinux related list of people, formatted in a particular way.
func nickToNameAndEmailWithUrl(nick string, url string) (string, os.Error) {
	var bval []byte
	tokerror := os.NewError("Out of tokens")
	tokenizer, body := getWebPageTokenizer(url)
	defer body.Close()
	for {
		toktype := tokenizer.Next()
		if toktype == html.ErrorToken {
			return "", tokerror
		}
		bval, _ = tokenizer.TagName()
		tagname := bytes.NewBuffer(bval).String()
		if tagname == "a" {
			// Find Name
			text := ""
			for text != "Name:" {
				if !Skip(tokenizer, 1) {
					return "", tokerror
				}
				bval = tokenizer.Text()
				text = bytes.NewBuffer(bval).String()
			}
			if !Skip(tokenizer, 4) {
				return "", tokerror
			}
			bval = tokenizer.Text()
			name := bytes.NewBuffer(bval).String()
			// Find Alias
			text = ""
			for text != "Alias:" {
				if !Skip(tokenizer, 1) {
					return "", tokerror
				}
				bval = tokenizer.Text()
				text = bytes.NewBuffer(bval).String()
			}
			if !Skip(tokenizer, 4) {
				return "", tokerror
			}
			bval = tokenizer.Text()
			alias := bytes.NewBuffer(bval).String()
			// Is there a space in the alias?
			if strings.Index(alias, " ") != -1 {
				// Split into two substrings, then only use the first part
				alias = strings.SplitN(alias, " ", 2)[0]
			}
			if (strings.ToLower(alias) != strings.ToLower(nick)) && (nick != generateNick(name)) {
				// Skipping this person if alias and nick doesn't match
				continue
			}
			// Find Email
			text = ""
			for text != "Email:" {
				if !Skip(tokenizer, 1) {
					return "", tokerror
				}
				bval = tokenizer.Text()
				text = bytes.NewBuffer(bval).String()
			}
			if !Skip(tokenizer, 4) {
				return "", tokerror
			}
			bval = tokenizer.Text()
			email := bytes.NewBuffer(bval).String()
			// If there's no "@" in the email, replace the first "." with "@"
			if strings.Index(email, "@") == -1 {
				if strings.Count(email, ".") > 1 {
					email = strings.Replace(email, ".", "@", 1)
				}
			}
			// Format the name and email nicely, then return
			return fmt.Sprintf("%s <%s>", name, email), nil
		}
	}
	return "", tokerror
}

func nickToNameAndEmail(nick string) string {
	if nickCache == nil {
		nickCache = make(map[string]string)
	}
	for key, value := range nickCache {
		if key == nick {
			return value
		}
	}
	// Try searching on the trusted user webpage
	nameEmail, err := nickToNameAndEmailWithUrl(nick, TU_URL)
	if err != nil {
		// Try searching on the developer webpage
		nameEmail, err = nickToNameAndEmailWithUrl(nick, DEV_URL)
		if err != nil {
			// Could not get name and email from nick
			nickCache[nick] = nick
			return nick
		}
	}
	nickCache[nick] = nameEmail
	return nameEmail
}

// Output the N last svn log entries in the style of a ChangeLog
func outputLog(n int) {
	leadStar := "    * "
	svnlog, err := getSvnLog(n)
	if err != nil {
		log.Println("Could not get svn log")
	}
	var date, prevdate, name, msg string
	for _, logentry := range svnlog.LogEntry {
		date = prettyDate(logentry.Date)
		name = nickToNameAndEmail(logentry.Author)
		msg = strings.TrimSpace(logentry.Msg)
		if msg == "" {
			// Skip empty messages
			continue
		}
		msg = leadStar + msg
		// If there are newlines in the msg, indent them
		msg = strings.Replace(msg, "\n", "\n      ", -1)
		// Only output a header if it's not the same date again
		if date != prevdate {
			// Don't start with a blank line first time
			if "" != prevdate {
				fmt.Println()
			}
			// Output header
			fmt.Printf("%s %s\n", date, name)
		}
		// Output message
		fmt.Println(msg)
		prevdate = date
	}
	// Output a last blank line, if we ever outputted anything
	if "" != date {
		fmt.Println()
	}
}

func main() {
	version_text := "svnchangelog " + VERSION
	help_text := "this brief help"

	flag.Usage = func() {
		fmt.Println()
		fmt.Println("Generates a ChangeLog based on \"svn log\".")
		fmt.Println()
		fmt.Println("Syntax:")
		fmt.Println("\tsvnchangelog [n]")
		fmt.Println()
		fmt.Println("Arguments:")
		fmt.Println("\tn - the number of entries to fetch from the log")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("\tsvnchangelog")
		fmt.Println("\tsvnchangelog 10")
		fmt.Println()
	}
	var missing_args = func() {
		fmt.Fprintf(os.Stderr, "Please provide an int that represents the number of svn log entries to recall.\nUse --help for more info.\n")
		os.Exit(1)
	}
	var version_long *bool = flag.Bool("version", false, version_text)
	var version_short *bool = flag.Bool("v", false, version_text)
	var help_long *bool = flag.Bool("help", false, help_text)
	var help_short *bool = flag.Bool("h", false, help_text)
	flag.Parse()

	version := *version_long || *version_short
	help := *help_long || *help_short

	args := flag.Args()

	if help {
		flag.Usage()
	} else if version {
		fmt.Println(VERSION)
	} else if len(args) == 1 {
		n, err := strconv.Atoi(args[0])
		if err != nil || n <= 0 {
			missing_args()
		} else {
			outputLog(n)
		}
	} else {
		outputLog(-1)
	}
}
