/*
https://core.telegram.org/bots/api/

GoFmt GoBuildNull GoPublish

heroku git:clone -a tgzeposter $HOME/tgzeposter/ && cd $HOME/tgzeposter/
heroku buildpacks:set https://github.com/ryandotsmith/null-buildpack.git
heroku addons:create scheduler:standard
heroku addons:attach scheduler-xyz
go get -a -u -v
GOOS=linux GOARCH=amd64 go build -trimpath -o $HOME/tgzeposter/ && cp tgzeposter.go go.mod go.sum *.text $HOME/tgzeposter/
cd $HOME/tgzeposter/ && git commit -am tgzeposter
cd $HOME/tgzeposter/ && git reset `{git commit-tree 'HEAD^{tree}' -m 'tgzeposter'}
cd $HOME/tgzeposter/ && git push -f
*/

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	dotenv "github.com/joho/godotenv"
)

func beats(td time.Duration) int {
	const beat = time.Duration(24) * time.Hour / 1000
	return int(td / beat)
}

func ts() string {
	tzBiel := time.FixedZone("Biel", 60*60)
	t := time.Now().In(tzBiel)
	ts := fmt.Sprintf("%03d/%s@%d", t.Year()%1000, t.Format("0102"), beats(time.Since(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, tzBiel))))
	return ts
}

func tsversion() string {
	tzBiel := time.FixedZone("Biel", 60*60)
	t := time.Now().In(tzBiel)
	v := fmt.Sprintf("%03d.%s.%d", t.Year()%1000, t.Format("0102"), beats(time.Since(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, tzBiel))))
	return v
}

func log(msg interface{}, args ...interface{}) {
	fmt.Fprintf(os.Stderr, ts()+" "+fmt.Sprintf("%s", msg)+NL, args...)
}

const (
	NL = "\n"

	DotenvPath = "tgposter.env"
)

var (
	Ctx        context.Context
	HttpClient = &http.Client{}

	TgToken    string
	TgOffset   int64
	ZeTgChatId int64

	HerokuToken   string
	HerokuVarsUrl string

	MoonPhaseTgChatId  int64
	MoonPhaseTodayLast string

	ABookOfDaysPath     string
	ABookOfDaysLast     string
	ABookOfDaysTgChatId int64
	ABookOfDaysRe       *regexp.Regexp

	ACourseInMiraclesWorkbookPath     string
	ACourseInMiraclesWorkbookLast     string
	ACourseInMiraclesWorkbookTgChatId int64
	ACourseInMiraclesWorkbookReString = "^\\* LESSON "
	ACourseInMiraclesWorkbookRe       *regexp.Regexp
)

type TgResponse struct {
	Ok          bool       `json:"ok"`
	Description string     `json:"description"`
	Result      *TgMessage `json:"result"`
}

type TgResponseShort struct {
	Ok          bool   `json:"ok"`
	Description string `json:"description"`
}

type TgMessage struct {
	MessageId int64 `json:"message_id"`
	Text      string
}

func getJson(url string, target interface{}) error {
	r, err := HttpClient.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	return json.NewDecoder(r.Body).Decode(target)
}

func postJson(url string, data *bytes.Buffer, target interface{}) error {
	resp, err := HttpClient.Post(
		url,
		"application/json",
		data,
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody := bytes.NewBuffer(nil)
	_, err = io.Copy(respBody, resp.Body)
	if err != nil {
		return fmt.Errorf("io.Copy: %v", err)
	}

	err = json.NewDecoder(respBody).Decode(target)
	if err != nil {
		return fmt.Errorf("Decode: %v", err)
	}

	return nil
}

func Setenv(name, value string) error {
	if HerokuVarsUrl != "" && HerokuToken != "" {
		return HerokuSetenv(name, value)
	}

	env, err := dotenv.Read(DotenvPath)
	if err != nil {
		log("WARNING: loading dotenv file: %v", err)
		env = make(map[string]string)
	}
	env[name] = value
	if err = dotenv.Write(env, DotenvPath); err != nil {
		return err
	}

	return nil
}

func HerokuSetenv(name, value string) error {
	req, err := http.NewRequest(
		"PATCH",
		HerokuVarsUrl,
		strings.NewReader(fmt.Sprintf(`{"%s": "%s"}`, name, value)),
	)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/vnd.heroku+json; version=3")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", HerokuToken))
	req.Header.Set("Content-Type", "application/json")
	resp, err := HttpClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("response status: %s", resp.Status)
	}

	return nil
}

func init() {
	var err error

	Ctx = context.TODO()

	if err = dotenv.Overload(DotenvPath); err != nil {
		log("WARNING: loading dotenv file: %v", err)
	}

	if os.Getenv("TgToken") != "" {
		TgToken = os.Getenv("TgToken")
	}
	if TgToken == "" {
		log("ERROR: TgToken empty")
		os.Exit(1)
	}
	if os.Getenv("TgOffset") != "" {
		TgOffset, err = strconv.ParseInt(os.Getenv("TgOffset"), 10, 0)
		if err != nil {
			log("ERROR: invalid TgOffset: %v", err)
			os.Exit(1)
		}
	}
	if os.Getenv("ZeTgChatId") == "" {
		log("ERROR: ZeTgChatId empty")
		os.Exit(1)
	} else {
		ZeTgChatId, err = strconv.ParseInt(os.Getenv("ZeTgChatId"), 10, 0)
		if err != nil {
			log("ERROR: invalid ZeTgChatId: %v", err)
			os.Exit(1)
		}
	}

	HerokuToken = os.Getenv("HerokuToken")
	if HerokuToken == "" {
		log("WARNING: HerokuToken empty")
	}
	HerokuVarsUrl = os.Getenv("HerokuVarsUrl")
	if HerokuVarsUrl == "" {
		log("WARNING: HerokuVarsUrl empty")
	}

	if os.Getenv("MoonPhaseTgChatId") == "" {
		MoonPhaseTgChatId = ZeTgChatId
	} else {
		MoonPhaseTgChatId, err = strconv.ParseInt(os.Getenv("MoonPhaseTgChatId"), 10, 0)
		if err != nil {
			log("ERROR: invalid MoonPhaseTgChatId: %v", err)
			os.Exit(1)
		}
	}
	MoonPhaseTodayLast = os.Getenv("MoonPhaseTodayLast")

	if os.Getenv("ABookOfDaysPath") != "" {
		ABookOfDaysPath = os.Getenv("ABookOfDaysPath")
		if os.Getenv("ABookOfDaysRe") == "" {
			log("ABookOfDaysRe env is empty")
			os.Exit(1)
		} else {
			ABookOfDaysRe = regexp.MustCompile(os.Getenv("ABookOfDaysRe"))
		}
		if os.Getenv("ABookOfDaysTgChatId") == "" {
			log("ABookOfDaysTgChatId env is empty")
			os.Exit(1)
		}
	}
	ABookOfDaysLast = os.Getenv("ABookOfDaysLast")
	if os.Getenv("ABookOfDaysTgChatId") != "" {
		ABookOfDaysTgChatId, err = strconv.ParseInt(os.Getenv("ABookOfDaysTgChatId"), 10, 0)
		if err != nil {
			log("ERROR: invalid ABookOfDaysTgChatId: %v", err)
			os.Exit(1)
		}
	}

	if os.Getenv("ACourseInMiraclesWorkbookPath") != "" {
		ACourseInMiraclesWorkbookPath = os.Getenv("ACourseInMiraclesWorkbookPath")
		if os.Getenv("ACourseInMiraclesWorkbookTgChatId") == "" {
			log("ACourseInMiraclesWorkbookTgChatId env is empty")
			os.Exit(1)
		}
	}
	ACourseInMiraclesWorkbookRe = regexp.MustCompile(ACourseInMiraclesWorkbookReString)
	ACourseInMiraclesWorkbookLast = os.Getenv("ACourseInMiraclesWorkbookLast")
	if os.Getenv("ACourseInMiraclesWorkbookTgChatId") != "" {
		ACourseInMiraclesWorkbookTgChatId, err = strconv.ParseInt(os.Getenv("ACourseInMiraclesWorkbookTgChatId"), 10, 0)
		if err != nil {
			log("ERROR: invalid ACourseInMiraclesWorkbookTgChatId: %v", err)
			os.Exit(1)
		}
	}
}

func PostACourseInMiraclesWorkbook() error {
	if ACourseInMiraclesWorkbookPath == "" || time.Now().UTC().Hour() < 4 {
		return nil
	}

	if time.Now().UTC().Month() == 3 && time.Now().UTC().Day() == 1 {
		ACourseInMiraclesWorkbookLast = ""
	}

	var ty0 time.Time
	if time.Now().UTC().Month() < 3 {
		ty0 = time.Date(time.Now().UTC().Year()-1, time.Month(3), 1, 0, 0, 0, 0, time.UTC)
	} else {
		ty0 = time.Date(time.Now().UTC().Year(), time.Month(3), 1, 0, 0, 0, 0, time.UTC)
	}
	daynum := time.Since(ty0)/(24*time.Hour) + 1
	daynums := fmt.Sprintf(" %d ", daynum)

	acimwbbb, err := ioutil.ReadFile(ACourseInMiraclesWorkbookPath)
	if err != nil {
		return fmt.Errorf("ReadFile ACourseInMiraclesWorkbookPath=`%s`: %v", ACourseInMiraclesWorkbookPath, err)
	}
	acimwb := string(acimwbbb)
	if acimwb == "" {
		return fmt.Errorf("Empty file ACourseInMiraclesWorkbookPath=`%s`", ACourseInMiraclesWorkbookPath)
	}
	acimwbss := strings.Split(acimwb, "\n\n\n\n")

	/*
	var longis []string
	for _, t := range acimwbss {
		if len(t) >= 4000 {
			tt := strings.Split(t, "\n")[0]
			longis = append(longis, tt)
		}
	}
	log("ACourseInMiraclesWorkbook texts of 4000+ length: %s", strings.Join(longis, ", "))
	*/

	if strings.Contains(ACourseInMiraclesWorkbookLast, daynums) {
		return nil
	}

	var skip bool
	if ACourseInMiraclesWorkbookLast != "" {
		skip = true
	}

	for _, s := range acimwbss {
		st := strings.Split(s, "\n")[0]
		if st == ACourseInMiraclesWorkbookLast {
			skip = false
			continue
		}
		if skip {
			continue
		}

		var spp []string
		if len(s) < 4000 {
			spp = append(spp, s)
		} else {
			var sp string
			srs := strings.Split(s, "\n\n")
			for sri, sr := range srs {
				sp += sr + "\n\n"
				if len(sp) > 3000 || sri == len(srs)-1 {
					spp = append(spp, sp)
					sp = ""
				}
			}
		}

		for i, sp := range spp {
			message := sp
			if i > 0 {
				message = st + " (continued)\n\n" + sp
			}
			_, err = tgsendMessage(message, ACourseInMiraclesWorkbookTgChatId, "MarkdownV2")
			if err != nil {
				return fmt.Errorf("tgsendMessage: %v", err)
			}
		}

		ACourseInMiraclesWorkbookLast = st

		err = Setenv("ACourseInMiraclesWorkbookLast", ACourseInMiraclesWorkbookLast)
		if err != nil {
			return fmt.Errorf("Setenv ACourseInMiraclesWorkbookLast: %v", err)
		}

		if ACourseInMiraclesWorkbookRe.MatchString(st) {
			break
		}
	}

	return nil
}

func PostABookOfDays() error {
	if ABookOfDaysPath == "" || time.Now().UTC().Hour() < 4 {
		return nil
	}

	abodbb, err := ioutil.ReadFile(ABookOfDaysPath)
	if err != nil {
		return fmt.Errorf("ReadFile ABookOfDaysPath=`%s`: %v", ABookOfDaysPath, err)
	}
	abod := string(abodbb)
	if abod == "" {
		return fmt.Errorf("Empty file ABookOfDaysPath=`%s`", ABookOfDaysPath)
	}

	monthday := time.Now().UTC().Format("January 2")
	if os.Getenv("ABookOfDaysRe") == "" {
		return fmt.Errorf("ABookOfDaysRe env is empty")
	}
	ABookOfDaysRe := regexp.MustCompile(strings.ReplaceAll(os.Getenv("ABookOfDaysRe"), "monthday", monthday))
	abodtoday := ABookOfDaysRe.FindString(abod)
	abodtoday = strings.TrimSpace(abodtoday)
	if abodtoday == "" {
		log("Could not find A Book of Days text for today")
		return nil
	}

	//log("abodtoday:\n%s", abodtoday)

	if monthday == ABookOfDaysLast {
		return nil
	}

	_, err = tgsendMessage(abodtoday, ABookOfDaysTgChatId, "MarkdownV2")
	if err != nil {
		return fmt.Errorf("tgsendMessage: %v", err)
	}

	err = Setenv("ABookOfDaysLast", monthday)
	if err != nil {
		return fmt.Errorf("Setenv ABookOfDaysLast: %v", err)
	}

	return nil
}

func MoonPhaseCalendar() string {
	nmfm := []string{"○", "●"}
	const MoonCycleDur time.Duration = 2551443 * time.Second
	var NewMoon time.Time = time.Date(2020, time.December, 14, 16, 16, 0, 0, time.UTC)
	var sinceNM time.Duration = time.Since(NewMoon) % MoonCycleDur
	var lastNM time.Time = time.Now().UTC().Add(-sinceNM)
	var msg, year, month string
	var mo time.Time = lastNM
	for i := 0; mo.Before(lastNM.Add(time.Hour * 24 * 7 * 54)); i++ {
		if mo.Format("2006") != year {
			year = mo.Format("2006")
			msg += NL + NL + fmt.Sprintf("Year %s", year) + NL
		}
		if mo.Format("Jan") != month {
			month = mo.Format("Jan")
			msg += NL + fmt.Sprintf("%s ", month)
		}
		msg += fmt.Sprintf(
			"%s:%s ",
			mo.Add(-4*time.Hour).Format("Mon/2"),
			nmfm[i%2],
		)
		mo = mo.Add(MoonCycleDur / 2)
	}
	return msg
}

func MoonPhaseToday() string {
	const MoonCycleDur time.Duration = 2551443 * time.Second
	var NewMoon time.Time = time.Date(2020, time.December, 14, 16, 16, 0, 0, time.UTC)
	var sinceNew time.Duration = time.Since(NewMoon) % MoonCycleDur
	var tnow time.Time = time.Now().UTC()
	if tillNew := MoonCycleDur - sinceNew; tillNew < 24*time.Hour {
		return fmt.Sprintf(
			"Today %s is New Moon; next Full Moon is on %s.",
			tnow.Format("Monday, January 2"),
			tnow.Add(MoonCycleDur/2).Format("Monday, January 2"),
		)
	}
	if tillFull := MoonCycleDur/2 - sinceNew; tillFull >= 0 && tillFull < 24*time.Hour {
		return fmt.Sprintf(
			"Today %s is Full Moon; next New Moon is on %s.",
			tnow.Format("Monday, January 2"),
			tnow.Add(MoonCycleDur/2).Format("Monday, January 2"),
		)
	}
	return ""
}

func PostMoonPhaseToday() error {
	var err error

	if time.Now().UTC().Hour() < 4 {
		return nil
	}

	moonphase := MoonPhaseToday()

	yearmonthday := time.Now().UTC().Format("2006 January 2")
	if yearmonthday == MoonPhaseTodayLast {
		return nil
	}

	if moonphase != "" {
		_, err = tgsendMessage(moonphase, MoonPhaseTgChatId, "MarkdownV2")
		if err != nil {
			return fmt.Errorf("tgsendMessage: %v", err)
		}
	}

	err = Setenv("MoonPhaseTodayLast", yearmonthday)
	if err != nil {
		return fmt.Errorf("Setenv MoonPhaseTodayLast: %v", err)
	}

	return nil
}

func tgsendMessage(text string, chatid int64, parsemode string) (msg *TgMessage, err error) {
	// https://core.telegram.org/bots/api/#sendmessage
	// https://core.telegram.org/bots/api/#formatting-options
	if parsemode == "MarkdownV2" {
		for _, c := range []string{`[`, `]`, `(`, `)`, `~`, "`", `>`, `#`, `+`, `-`, `=`, `|`, `{`, `}`, `.`, `!`} {
			text = strings.ReplaceAll(text, c, `\`+c)
		}
		text = strings.ReplaceAll(text, "______", `\_\_\_\_\_\_`)
		text = strings.ReplaceAll(text, "_____", `\_\_\_\_\_`)
		text = strings.ReplaceAll(text, "____", `\_\_\_\_`)
		text = strings.ReplaceAll(text, "___", `\_\_\_`)
		text = strings.ReplaceAll(text, "__", `\_\_`)
	}
	sendMessage := map[string]interface{}{
		"chat_id":                  chatid,
		"text":                     text,
		"parse_mode":               parsemode,
		"disable_web_page_preview": true,
	}
	sendMessageJSON, err := json.Marshal(sendMessage)
	if err != nil {
		return nil, err
	}

	var tgresp TgResponse
	err = postJson(
		fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", TgToken),
		bytes.NewBuffer(sendMessageJSON),
		&tgresp,
	)
	if err != nil {
		return nil, err
	}

	if !tgresp.Ok {
		return nil, fmt.Errorf("sendMessage: %s", tgresp.Description)
	}

	msg = tgresp.Result

	return msg, nil
}

func main() {
	var err error

	err = PostMoonPhaseToday()
	if err != nil {
		log("WARNING: PostMoonPhaseToday: %v", err)
	}

	err = PostABookOfDays()
	if err != nil {
		log("WARNING: PostABookOfDays: %v", err)
	}

	err = PostACourseInMiraclesWorkbook()
	if err != nil {
		log("WARNING: PostACourseInMiraclesWorkbook: %v", err)
	}

	return
}
