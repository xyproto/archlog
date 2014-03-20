/*
 * Program for generating a ChangeLog based on svn log.
 *
 * Alexander Rødseth <rodseth@gmail.com>
 *
 * GPL2
 *
 * 2012-01-14
 * 2012-01-29
 * 2012-07-12
 * 2013-08-22
 * 2014-03-20
 *
 */

package main

import (
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/scanner"
	"io/ioutil"
)

const (
	VERSION = "0.5"
	TU_URL  = "http://www.archlinux.org/trustedusers/"
	DEV_URL = "http://www.archlinux.org/developers/"
	FEL_URL = "http://www.archlinux.org/fellows/"
	PKG_URL = "http://www.archlinux.org/packages/"
)

// Used when parsing svn log xml
type LogEntry struct {
	Revision string `xml:"revision,attr"`
	Author   string `xml:"author"`
	Date     string `xml:"date"`
	Msg      string `xml:"msg"`
}

// Used when parsing svn log xml
type LogEntries struct {
	XMLName  xml.Name   `xml:"log"`
	LogEntry []LogEntry `xml:"logentry"`
}

var (
	nickCache map[string]string
)

// Get the xvn log xml output as an array of bytes
func getSvnLogXMLbytes(entries int) ([]byte, error) {
	var cmd *exec.Cmd
	if entries == -1 {
		// Get the entries in reverse order by asking for revisions from HEAD to 0
		cmd = exec.Command("/usr/bin/svn", "log", "--xml", "-r", "HEAD:0")
	} else {
		entriesText := fmt.Sprintf("%v", entries)
		// Get the entries in reverse order by asking for revisions from HEAD to 0
		cmd = exec.Command("/usr/bin/svn", "log", "--xml", "-r", "HEAD:0", "--limit", entriesText)
	}
	b, err := cmd.Output()
	if err != nil {
		// Return an error
		return []byte{}, err
	}
	return b, nil
}

// Use the "svn log --xml" command to fetch log entries
func getSvnLog(entries int) (LogEntries, error) {
	xmlbytes, err := getSvnLogXMLbytes(entries)

	//fmt.Println(string(xmlbytes))

	result := LogEntries{}
	err = xml.Unmarshal(xmlbytes, &result)
	if err != nil {
		fmt.Printf("error: %v", err)
		return LogEntries{}, nil
	}

	return result, nil
}

// Make a date from the xml version of svn log somewhat prettier
func prettyDate(date string) string {
	return strings.Split(date, "T")[0]
}

// Get the contents from an URL and return a tokenizer and a ReadCloser
func getWebPageTokenizer(url string) (*scanner.Scanner, io.ReadCloser) {
	var client http.Client
	resp, err := client.Get(url)
	if err != nil {
		log.Println("Could not retrieve " + url)
		return nil, nil
	}
	var tokenizer scanner.Scanner
	tokenizer.Init(resp.Body)
	return &tokenizer, resp.Body
}

// Skip N tokens, if possible. Returns true if it worked out.
func Skip(tokenizer *scanner.Scanner, n int) bool {
	for counter := 0; counter < n; counter++ {
		toktype := tokenizer.Next()
		if toktype == scanner.EOF {
			return false
		}
	}
	return true
}

// TODO: Find a better way
func mapRunes(letter rune) rune {
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
	nick := strings.Replace(strings.ToLower(strings.Map(mapRunes, string(firstname[0])+lastname)), "_", "", -1)
	return nick
}

// Find the name and email based on a nick name and an URL to an
// ArchLinux related list of people, formatted in a particular way.
func nickToNameAndEmailWithUrl(nick string, url string) (string, error) {
	var client http.Client
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	name := ""
	email := ""
	counter := 0
	email_index := -1
	found := false
	for i, tag := range strings.Split(string(b), "<") {
		if strings.Contains(tag, "schema.org/Person") {
			name = ""
			email = ""
			counter = 30 // Examine the next 30 tags
		}
		if counter > 0 {
			if strings.Contains(tag, "itemprop=\"name") && !strings.Contains(tag, "Arch Linux") {
				name = strings.Split(tag, "\"")[3]
			} else if strings.Contains(tag, nick) {
				found = true
			} else if strings.Contains(tag, "Email") {
				email_index = i + 2
			} else if i == email_index {
				email = strings.Split(tag, ">")[1]
				// If there's no "@" in the email, replace the first "." with "@"
				if !strings.Contains(email, "@") && strings.Contains(email, ".") {
					email = strings.Replace(email, ".", "@", 1)
				}
				if found {
					break
				}
			}
			counter--
		}
	}
	if found {
		//fmt.Println("FOUND!")
		//fmt.Println("NICK", nick)
		//fmt.Println("EMAIL", email)
		//fmt.Println("NAME", name)
		// Format the name and email nicely, then return
		return fmt.Sprintf("%s <%s>", name, email), nil
	}
	return "", errors.New("Could not find nick")
}

// Find the name from an ArchLinux related list of people and nicks
func nickToNameFromListBox(nick string, url string) (string, error) {
	tokerror := errors.New("Out of tokens")
	tokenizer, body := getWebPageTokenizer(url)
	defer body.Close()
	for {
		if !Skip(tokenizer, 1) {
			return "", tokerror
		}
		tagname := tokenizer.TokenText() // TagName()
		if tagname == "option" {
			// Find Nick
			foundnick := tokenizer.TokenText() // TagAttr()
			if nick != foundnick {
				continue
			}
			if !Skip(tokenizer, 1) {
				return "", tokerror
			}
			name := tokenizer.TokenText()
			return name, nil
		}
	}
	return "", tokerror
}

// Find the email based on a name and an URL to an
// ArchLinux related list of people, formatted in a particular way.
func nameToEmailWithUrl(fullname string, url string) (string, error) {
	tokerror := errors.New("Out of tokens")
	tokenizer, body := getWebPageTokenizer(url)
	defer body.Close()
	for {
		if !Skip(tokenizer, 1) {
			return "", tokerror
		}
		tagname := tokenizer.TokenText() // TagName?
		if tagname == "a" {
			// Find Name
			text := ""
			for text != "Name:" {
				if !Skip(tokenizer, 1) {
					return "", tokerror
				}
				text = tokenizer.TokenText()
			}
			if !Skip(tokenizer, 4) {
				return "", tokerror
			}
			name := tokenizer.TokenText()
			// Check if this is the one we're looking for or skip
			if strings.ToLower(name) != strings.ToLower(fullname) {
				// Skipping this person if names doesn't match
				continue
			}
			// Find Alias
			text = ""
			for text != "Alias:" {
				if !Skip(tokenizer, 1) {
					return "", tokerror
				}
				text = tokenizer.TokenText()
			}
			if !Skip(tokenizer, 4) {
				return "", tokerror
			}
			_ = tokenizer.TokenText()
			//alias := bytes.NewBuffer(bval).String()
			// Find Email
			text = ""
			for text != "Email:" {
				if !Skip(tokenizer, 1) {
					return "", tokerror
				}
				text = tokenizer.TokenText()
			}
			if !Skip(tokenizer, 4) {
				return "", tokerror
			}
			email := tokenizer.TokenText()
			// If there's no "@" in the email, replace the first "." with "@"
			if strings.Index(email, "@") == -1 {
				if strings.Count(email, ".") > 1 {
					email = strings.Replace(email, ".", "@", 1)
				}
			}
			// Return the email and no error
			return email, nil
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
	if err == nil {
		// Found it
		nickCache[nick] = nameEmail
		return nameEmail
	}
	// Try searching on the developer webpage
	nameEmail, err = nickToNameAndEmailWithUrl(nick, DEV_URL)
	if err == nil {
		// Found it
		nickCache[nick] = nameEmail
		return nameEmail
	}
	// Try searching the package search webpage
	name, err := nickToNameFromListBox(nick, PKG_URL)
	if err == nil {
		// Found it, try to find the mail too
		var foundEmail bool = false
		var email string
		email, err = nameToEmailWithUrl(name, TU_URL)
		if err == nil {
			foundEmail = true
		} else {
			email, err = nameToEmailWithUrl(name, DEV_URL)
			if err == nil {
				foundEmail = true
			}
		}
		if foundEmail {
			name = fmt.Sprintf("%s <%s>", name, email)
		}
		nickCache[nick] = name
		return name
	}
	// Try searching on the fellows webpage
	nameEmail, err = nickToNameAndEmailWithUrl(nick, FEL_URL)
	if err == nil {
		// Found it
		nickCache[nick] = nameEmail
		return nameEmail
	}
	// Could not get name and email from nick
	nickCache[nick] = nick
	return nick
}

func abs(x int) int {
	if x >= 0 {
		return x
	}
	return -x
}

// Output the N last svn log entries in the style of a ChangeLog
func outputLog(n int) {
	first := true
	msgitems := make([]string, 0, abs(n))
	leadStar := "    * "
	svnlog, err := getSvnLog(n)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not find a subversion repository here")
		os.Exit(1)
	}
	var date, prevdate, name, prevname, msg, prevheader, header string
	for _, logentry := range svnlog.LogEntry {
		date = prettyDate(logentry.Date)
		name = nickToNameAndEmail(logentry.Author)
		msg = strings.TrimSpace(logentry.Msg)
		header = fmt.Sprintf("%s %s", date, name)
		if msg == "" {
			// Skip empty messages
			continue
		}
		msg = leadStar + msg
		// Where there is one blank line, remove it
		if strings.Count(msg, "\n\n") == 1 {
			msg = strings.Replace(msg, "\n\n", "\n", 1)
		}
		// If there are newlines in the msg, indent them
		msg = strings.Replace(msg, "\n", "\n      ", -1)
		// Only output a header if it's not the same date again, or not the same name
		if (date != prevdate) || (name != prevname) {
			// Output gathered messages
			if len(msgitems) > 0 {
				// Don't start with a blank line first time
				if "" != prevdate {
					if !first {
						//fmt.Println()
					}
				}
				// Output in reverse order
				last := len(msgitems) - 1
				for i, _ := range msgitems {
					fmt.Println(msgitems[last-i])
				}
				// Clear the gathered messages
				msgitems = []string{}
				first = false
			}
		}
		// Output a new header if it changes
		if !first && (header != prevheader) {
			fmt.Println("\n" + header)
		} else if first && (header != prevheader) {
			fmt.Println(header)
		}
		// Gather message
		msgitems = append(msgitems, msg)
		prevdate = date
		prevname = name
		prevheader = header
	}
	// Output any final gathered messages
	if len(msgitems) > 0 {
		// Output in reverse order
		last := len(msgitems) - 1
		for i, _ := range msgitems {
			fmt.Println(msgitems[last-i])
		}
		fmt.Println()
	}
}

func main() {
	version_text := "archlog " + VERSION
	help_text := "this brief help"

	flag.Usage = func() {
		fmt.Println()
		fmt.Println("Generates a ChangeLog based on \"svn log\".")
		fmt.Println("Tries to find names and e-mail addresses for Arch Linux related usernames")
		fmt.Println()
		fmt.Println("Syntax:")
		fmt.Println("\tarchlog [n]")
		fmt.Println()
		fmt.Println("Arguments:")
		fmt.Println("\tn - the number of entries to fetch from the log")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("\tarchlog")
		fmt.Println("\tarchlog 10")
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
