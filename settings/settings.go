// Package settings reads and writes to an MDroid settings file
package settings

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/MrDoctorKovacic/MDroid-Core/formatting"
	"github.com/MrDoctorKovacic/MDroid-Core/gps"
	"github.com/MrDoctorKovacic/MDroid-Core/influx"
	"github.com/MrDoctorKovacic/MDroid-Core/logging"
	"github.com/tarm/serial"

	"github.com/gorilla/mux"
)

// ConfigValues controls program settings and general persistent settings
type ConfigValues struct {
	BluetoothAddress      string
	DB                    *influx.Influx
	HardwareSerialEnabled bool
	HardwareSerialPort    string
	HardwareSerialBaud    string
	Location              *gps.Location
	SerialControlDevice   *serial.Port
	SettingsFile          string
	SlackURL              string
	VerboseOutput         bool
}

// settingsFile is the internal reference file for saving settings to
var settingsFile = "./settings.json"

// Settings control generic user defined field:value mappings, which will persist each run
// The mutex should be unnecessary, but is provided just in case
var Settings map[string]map[string]string
var settingsLock sync.Mutex

// SettingsStatus will control logging and reporting of status / warnings / errors
var SettingsStatus = logging.NewStatus("Settings")

// Configure verbose output
var verboseOutput bool

// HandleGetAll returns all current settings
func HandleGetAll(w http.ResponseWriter, r *http.Request) {
	if verboseOutput {
		SettingsStatus.Log(logging.OK(), "Responding to GET request with entire settings map.")
	}
	settingsLock.Lock()
	json.NewEncoder(w).Encode(formatting.JSONResponse{Output: Settings, Status: "success", OK: true})
	settingsLock.Unlock()
}

// HandleGet returns all the values of a specific setting
func HandleGet(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	componentName := formatting.FormatName(params["component"])

	if verboseOutput {
		SettingsStatus.Log(logging.OK(), fmt.Sprintf("Responding to GET request for setting component %s", componentName))
	}

	settingsLock.Lock()
	responseVal, ok := Settings[componentName]
	settingsLock.Unlock()

	var response formatting.JSONResponse
	if !ok {
		response = formatting.JSONResponse{Output: "Setting not found.", Status: "fail", OK: false}
	} else {
		response = formatting.JSONResponse{Output: responseVal, Status: "success", OK: true}
	}

	json.NewEncoder(w).Encode(response)
}

// HandleGetValue returns a specific setting value
func HandleGetValue(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	componentName := formatting.FormatName(params["component"])
	settingName := formatting.FormatName(params["name"])

	if verboseOutput {
		SettingsStatus.Log(logging.OK(), fmt.Sprintf("Responding to GET request for setting %s on component %s", settingName, componentName))
	}

	settingsLock.Lock()
	responseVal, ok := Settings[componentName][settingName]
	settingsLock.Unlock()

	var response formatting.JSONResponse
	if !ok {
		response = formatting.JSONResponse{Output: "Setting not found.", Status: "fail", OK: false}
	} else {
		response = formatting.JSONResponse{Output: responseVal, Status: "success", OK: true}
	}

	json.NewEncoder(w).Encode(response)
}

// Get returns all the values of a specific setting
func Get(componentName string, settingName string) (string, error) {
	componentName = formatting.FormatName(componentName)

	if verboseOutput {
		SettingsStatus.Log(logging.OK(), fmt.Sprintf("Responding to request for setting component %s", componentName))
	}

	settingsLock.Lock()
	defer settingsLock.Unlock()
	component, ok := Settings[componentName]
	if ok {
		setting, ok := component[settingName]
		if ok {
			return setting, nil
		}
	}
	return "", fmt.Errorf("Could not find component/setting with those values")
}

// HandleSet is the http wrapper for our setting setter
func HandleSet(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)

	// Parse out params
	componentName := formatting.FormatName(params["component"])
	settingName := formatting.FormatName(params["name"])
	settingValue := params["value"]

	// Log if requested
	if verboseOutput {
		SettingsStatus.Log(logging.OK(), fmt.Sprintf("Responding to POST request for setting %s on component %s to be value %s", settingName, componentName, settingValue))
	}

	// Do the dirty work elsewhere
	Set(componentName, settingName, settingValue)

	// Respond with OK
	json.NewEncoder(w).Encode(formatting.JSONResponse{Output: componentName, Status: "success", OK: true})
}

// Set will handle actually updates or posts a new setting value
func Set(componentName string, settingName string, settingValue string) {
	// Insert componentName into Map if not exists
	settingsLock.Lock()
	if _, ok := Settings[componentName]; !ok {
		Settings[componentName] = make(map[string]string, 0)
	}

	// Update setting in inner map
	Settings[componentName][settingName] = settingValue
	settingsLock.Unlock()

	// Log our success
	SettingsStatus.Log(logging.OK(), fmt.Sprintf("Updated setting %s[%s] to %s", componentName, settingName, settingValue))

	// Write out all settings to a file
	writeFile(settingsFile)
}
