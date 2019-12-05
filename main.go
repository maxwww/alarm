package main

import (
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/joho/godotenv"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	NANO_SECOND = 1000000000
	SECOND      = 1
	MINUTE      = 60
	HOUR        = 3600
	DAY         = 86400
)

var Suffixes = []Suffix{
	{"s", SECOND},
	{"m", MINUTE},
	{"h", HOUR},
	{"d", DAY},
}

var AlarmMap = map[int64][]Alarm{}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	token := os.Getenv("TOKEN")

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatal(err)
	}

	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("List"),
			tgbotapi.NewKeyboardButton("Clear all"),
		),
	)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		log.Fatal(err)
	}

	for update := range updates {
		go handleUpdate(update, bot, keyboard)
	}
}

func handleUpdate(update tgbotapi.Update, bot *tgbotapi.BotAPI, keyboard tgbotapi.ReplyKeyboardMarkup) {
	if update.Message == nil {
		return
	}
	log.Printf("[%s] %v - start", update.Message.From.UserName, update.Message.Text)
	defer log.Printf("[%s] %s - end", update.Message.From.UserName, update.Message.Text)
	responseMessage := ""

	switch {
	case update.Message.Text == "List":
		alarms, ok := AlarmMap[update.Message.Chat.ID]
		if !ok || len(alarms) == 0 {
			responseMessage = "List is empty"
		} else {
			now := time.Now().UnixNano()
			sort.Slice(alarms, func(i, j int) bool {
				return (int64(alarms[i].delay) - (now - alarms[i].time)) < (int64(alarms[j].delay) - (now - alarms[j].time))
			})
			responseMessage += "List"
			for _, v := range alarms {
				responseMessage += fmt.Sprintf("\n%s (%s)", SecondsToString((int64(v.delay)-(now-v.time))/NANO_SECOND), SecondsToString(int64(v.delay)/NANO_SECOND))
			}
		}
	case update.Message.Text == "Clear all":
		responseMessage = "Done"
		alarms, ok := AlarmMap[update.Message.Chat.ID]
		if ok {
			for _, v := range alarms {
				v.ch <- 1
			}
			AlarmMap[update.Message.Chat.ID] = nil
		}
	default:
		re := regexp.MustCompile(`(?i)\d+[smhd]?`)
		params := re.FindAll([]byte(strings.ToLower(update.Message.Text)), -1)

		if len(params) > 0 {
			var result int
			for _, v := range params {
				var seconds int
				sV := string(v)
				for _, suffix := range Suffixes {
					if strings.HasSuffix(sV, suffix.Suffix) {
						s, err := strconv.Atoi(strings.TrimSuffix(sV, suffix.Suffix))
						if err != nil {
							seconds = 0
						} else {
							seconds = s * suffix.Coef
						}
					}
				}
				if seconds == 0 {
					s, err := strconv.Atoi(sV)
					seconds = s * MINUTE
					if err != nil {
						seconds = 0
					}
				}

				result += seconds
			}

			ch := make(chan int)
			go sendWithDelay(result, update.Message.Chat.ID, ch, bot, keyboard)
			alarms, ok := AlarmMap[update.Message.Chat.ID]
			if !ok {
				alarms = []Alarm{}
			}
			AlarmMap[update.Message.Chat.ID] = append(alarms, Alarm{ch: ch, time: time.Now().UnixNano(), delay: result * NANO_SECOND})
			responseMessage = SecondsToString(int64(result))
		} else {
			responseMessage = update.Message.Text
		}
	}

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, responseMessage)
	msg.ReplyMarkup = keyboard
	_, err := bot.Send(msg)
	if err != nil {
		log.Print(err)
	}
}

func sendWithDelay(delay int, chatId int64, ch chan int, bot *tgbotapi.BotAPI, keyboard tgbotapi.ReplyKeyboardMarkup) {
	select {
	case <-ch:
		return
	case <-time.After(time.Duration(delay) * time.Second):
		msg := tgbotapi.NewMessage(chatId, "Alarm")
		msg.ReplyMarkup = keyboard
		_, err := bot.Send(msg)
		if err != nil {
			log.Print(err)
		}
		checkAlarmMap(chatId)
	}

}

func checkAlarmMap(chatId int64) {
	alarms, ok := AlarmMap[chatId]
	if !ok {
		return
	}
	var newAlarms []Alarm
	now := time.Now().UnixNano()
	for _, v := range alarms {
		if int64(v.delay)-(now-v.time) >= 0 {
			newAlarms = append(newAlarms, v)
		}
	}

	AlarmMap[chatId] = newAlarms
}

func SecondsToString(seconds int64) string {
	d, s := seconds/DAY, seconds%DAY
	h, s := s/HOUR, s%HOUR
	m, s := s/MINUTE, s%MINUTE

	var result string

	if d != 0 {
		result += fmt.Sprintf("%dd ", d)
	}

	if h != 0 || result != "" {
		result += fmt.Sprintf("%dh ", h)
	}

	if m != 0 || result != "" {
		result += fmt.Sprintf("%dm ", m)
	}

	result += fmt.Sprintf("%ds", s)

	return result
}

type Suffix struct {
	Suffix string
	Coef   int
}

type Alarm struct {
	ch    chan int
	time  int64
	delay int
}
