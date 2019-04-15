package sessions

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/MrDoctorKovacic/GoQMW/external/status"
	"github.com/MrDoctorKovacic/GoQMW/influx"
	"github.com/gorilla/mux"
)

// SessionData holds the name, data, last update info for each session value
type SessionData struct {
	Value      string `json:"value,omitempty"`
	LastUpdate string `json:"lastUpdate,omitempty"`
}

// GPSData holds various data points we expect to receive
type GPSData struct {
	Latitude  string   `json:"latitude,omitempty"`
	Longitude string   `json:"longitude,omitempty"`
	Time      string   `json:"time,omitempty"` // This will help measure latency :)
	Altitude  *float32 `json:"altitude,omitempty"`
	EPV       *float32 `json:"epv,omitempty"`
	EPT       *float32 `json:"ept,omitempty"`
	Speed     *float32 `json:"speed,omitempty"`
	Climb     *float32 `json:"climb,omitempty"`
}

// ALPRData holds the plate and percent for each new ALPR value
type ALPRData struct {
	Plate   string `json:"plate,omitempty"`
	Percent int    `json:"percent,omitempty"`
}

// Session is the global session accessed by incoming requests
var Session map[string]SessionData

// GPS is the last posted GPS fix
var GPS GPSData

// DB variable for influx
var DB influx.Influx

// SessionFile will designate where to output session to a file
var SessionFile string

// SessionStatus will control logging and reporting of status / warnings / errors
var SessionStatus = status.NewStatus("Session")

func init() {
	// Init session
	Session = make(map[string]SessionData)
}

// Setup provides global DB for future queries, other planned setup instructions as well
func Setup(database influx.Influx, file string) {
	DB = database
	SessionFile = file

	// Fetch and append old session from disk if allowed
	if SessionFile != "" {
		jsonFile, err := os.Open(SessionFile)

		if err != nil {
			SessionStatus.Log(status.Warning(), "Error opening JSON file on disk: "+err.Error())
		} else {
			byteValue, _ := ioutil.ReadAll(jsonFile)
			json.Unmarshal(byteValue, &Session)
		}
	} else {
		SessionStatus.Log(status.OK(), "Not saving or recovering from file")
	}
}

// GetSession returns the entire current session
func GetSession(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(Session)
}

// GetSessionValue returns a specific session value
func GetSessionValue(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	json.NewEncoder(w).Encode(Session[params["name"]])
}

// SetSessionValue updates or posts a new session value to the common session
func SetSessionValue(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	var newdata SessionData
	_ = json.NewDecoder(r.Body).Decode(&newdata)

	// Set last updated time to now
	var timestamp = time.Now().Format("2006-01-02 15:04:05.999")
	newdata.LastUpdate = timestamp

	// Add / update value in global session
	Session[params["name"]] = newdata

	// Insert into database
	err := DB.Write(fmt.Sprintf("pybus,name=%s value=\"%s\"", params["name"], newdata.Value))

	if err != nil {
		SessionStatus.Log(status.Error(), "Error writing "+params["name"]+"="+newdata.Value+" to influx DB: "+err.Error())
	} else {
		SessionStatus.Log(status.OK(), "Logged "+params["name"]+"="+newdata.Value+" to database")
	}

	// Respond with inserted values
	json.NewEncoder(w).Encode(newdata)
}

// SetGPSValue posts a new GPS fix
func SetGPSValue(w http.ResponseWriter, r *http.Request) {
	var newdata GPSData
	_ = json.NewDecoder(r.Body).Decode(&newdata)

	// Prepare new value
	var postingString strings.Builder

	// Update value for global session if the data is newer (not nil)
	// Can't find a better way to go about this
	if newdata.Latitude != "" {
		GPS.Latitude = newdata.Latitude
		postingString.WriteString(fmt.Sprintf("latitude=\"%s\",", newdata.Latitude))
	}
	if newdata.Longitude != "" {
		GPS.Longitude = newdata.Longitude
		postingString.WriteString(fmt.Sprintf("longitude=\"%s\",", newdata.Longitude))
	}
	if newdata.Altitude != nil {
		GPS.Altitude = newdata.Altitude
		log.Println(fmt.Sprintf("%f", *newdata.Altitude))
		postingString.WriteString(fmt.Sprintf("altitude=%f,", *newdata.Altitude))
	}
	if newdata.Speed != nil {
		GPS.Speed = newdata.Speed
		postingString.WriteString(fmt.Sprintf("speed=%f,", *newdata.Speed))
	}
	if newdata.Climb != nil {
		GPS.Climb = newdata.Climb
		postingString.WriteString(fmt.Sprintf("climb=%f,", *newdata.Climb))
	}
	if newdata.Time != "" {
		GPS.Time = newdata.Time
		// This is ultimately a waste of space
		//postingString.WriteString(fmt.Sprintf("timestamp=\"%s\",", newdata.Time))
	}
	if newdata.EPV != nil {
		GPS.EPV = newdata.EPV
		postingString.WriteString(fmt.Sprintf("EPV=%f,", *newdata.EPV))
	}
	if newdata.EPT != nil {
		GPS.EPT = newdata.EPT
		postingString.WriteString(fmt.Sprintf("EPT=%f,", *newdata.EPT))
	}

	// Insert into database
	err := DB.Write(fmt.Sprintf("gps %s", strings.TrimSuffix(postingString.String(), ",")))

	if err != nil {
		SessionStatus.Log(status.Error(), "Error writing string "+postingString.String()+" to influx DB: "+err.Error())
	} else {
		SessionStatus.Log(status.OK(), "Logged "+postingString.String()+" to database")
	}

	// Respond with inserted values
	json.NewEncoder(w).Encode("OK")
}

// LogALPR creates a new entry in running SQL DB
func LogALPR(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	decoder := json.NewDecoder(r.Body)
	var newplate ALPRData
	err := decoder.Decode(&newplate)

	if err != nil {
		log.Println(err)
	} else {
		// Decode plate/time/etc values
		plate := params["plate"]
		percent := newplate.Percent

		if plate != "" {
			// Insert into database
			err := DB.Write(fmt.Sprintf("alpr,plate=%s percent=%d", plate, percent))

			if err != nil {
				SessionStatus.Log(status.Error(), "Error writing "+plate+" to influx DB: "+err.Error())
			} else {
				SessionStatus.Log(status.OK(), "Logged "+plate+" to database")
			}
		} else {
			log.Println(fmt.Sprintf("Missing arguments, ignoring post of %s with percent of %d", plate, percent))
		}

		// Respond with inserted values
		json.NewEncoder(w).Encode(plate)
	}
}

// RestartALPR posts remote device to restart ALPR service
func RestartALPR(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode("OK")
}
