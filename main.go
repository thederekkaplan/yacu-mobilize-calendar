package main

import (
	"net/http"
	"context"
	"encoding/json"
	"io/ioutil"
	"time"
	"fmt"
	"os"
	"strconv"

	"google.golang.org/api/calendar/v3"
	"cloud.google.com/go/firestore"
)

type MobilizeEvent struct {
	Title string
	Description string
	Url string `json:"browser_url"`
	Timeslots []Timeslot
}

type Timeslot struct {
	Id float64
	StartDate float64 `json:"start_date"`
	EndDate float64 `json:"end_date"`
}

type Event struct {
	Id float64
	Title string
	Description string
	Url string
	StartDate time.Time
	EndDate time.Time
}

type Doc struct {
	ModifiedDate int `firestore:"modified_date"`
}

func main() {
	http.HandleFunc("/update", update)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Println("Listening on port", port)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		panic(err)
	}
}

func update(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("ENV") == "PROD" && r.Header.Get("X-Appengine-Cron") != "true" {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "403 Forbidden")
		return
	}

	ctx := context.Background()

	client, err := firestore.NewClient(ctx, "commanding-way-273100")
	if err != nil {
		panic(err)
	}

	doc := client.Doc("data/general")
	docsnap, err := doc.Get(ctx)
	if err != nil {
		panic(err)
	}

	var data Doc
	err = docsnap.DataTo(&data)
	if (err != nil) {
		panic(err)
	}

	modifiedDate := time.Now()
	events := getEvents("https://api.mobilize.us/v1/organizations/2596/events?updated_since=" + strconv.Itoa(data.ModifiedDate))
	saveEvents(events)

	if os.Getenv("ENV") == "PROD" {
		_, err = doc.Update(ctx, []firestore.Update{{Path: "modified_date", Value: modifiedDate.Unix()}})
		if err != nil {
			panic(err)
		}
	}

	fmt.Fprint(w, "Update Complete")
}

// Collects the events from the Mobilize API
func getEvents(url string) []Event {
	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	var objmap map[string]json.RawMessage
	err = json.Unmarshal(body, &objmap)
	if err != nil {
		panic(err)
	}

	var mobilizeEvents []MobilizeEvent

	err = json.Unmarshal(objmap["data"], &mobilizeEvents)
	if err != nil {
		panic(err)
	}

	var events []Event

	// Convert timeslots into separate events
	for _, event := range mobilizeEvents {
		for _, timeslot := range event.Timeslots {
			events = append(events, Event{
				Id: timeslot.Id,
				Title: event.Title,
				Description: event.Description,
				Url: event.Url,
				StartDate: time.Unix(int64(timeslot.StartDate), 0),
				EndDate: time.Unix(int64(timeslot.EndDate), 0),
			})
		}
	}

	return events
}

func saveEvents(events []Event) {
	svc, err := calendar.NewService(context.Background())
	if err != nil {
		panic(err)
	}

	cal := getCal(svc)

	for _, item := range events {
		event := &calendar.Event {
			Id: strconv.Itoa(int(item.Id)),
			Summary: item.Title,
			Description: item.Description,
			Location: item.Url,
			Start: &calendar.EventDateTime {
				DateTime: item.StartDate.Format(time.RFC3339),
			},
			End: &calendar.EventDateTime {
				DateTime: item.EndDate.Format(time.RFC3339),
			},
		}

		// Try to get event. If an error is not returned, update it. If an error is returned, it does not exist, so create it.
		_, err := svc.Events.Get(cal.Id, event.Id).Do()
		if err == nil {
			_, err = svc.Events.Update(cal.Id, event.Id, event).Do()
			if err != nil {
				panic(err)
			}
		} else {
			_, err = svc.Events.Insert(cal.Id, event).Do()
			if err != nil {
				panic(err)
			}
		}
	}
}

func getCal(svc *calendar.Service) *calendar.Calendar {
	list, err := svc.CalendarList.List().Do()
	if err != nil {
		panic(err)
	}

	var cal *calendar.Calendar
	if len(list.Items) == 0 {
		cal, err = svc.Calendars.Insert(&calendar.Calendar{
			Summary: "YACU Events",
		}).Do()
		if err != nil {
			panic(err)
		}

		_, err = svc.Acl.Insert(cal.Id, &calendar.AclRule {
			Role: "reader",
			Scope: &calendar.AclRuleScope {
				Type: "default",
			},
		}).Do()
		if err != nil {
			panic(err)
		}


		// In production, adds calendar ID to firestore. In development, prints it to the console.
		if os.Getenv("ENV") == "PROD" {
			ctx := context.Background()

			client, err := firestore.NewClient(ctx, "commanding-way-273100")
			if err != nil {
				panic(err)
			}

			doc := client.Doc("data/general")
			_, err = doc.Update(ctx, []firestore.Update{{Path: "calendar", Value: cal.Id}})
			if err != nil {
				panic(err)
			}

		} else {
			fmt.Println(cal.Id)
		}

	} else {
		id := list.Items[0].Id
		cal, err = svc.Calendars.Get(id).Do()
		if err != nil {
			panic(err)
		}
	}

	return cal 
}