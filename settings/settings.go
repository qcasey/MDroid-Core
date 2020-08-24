// Package settings reads and writes to an MDroid settings file
package settings

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/qcasey/MDroid-Core/format"
	"github.com/qcasey/MDroid-Core/format/response"
	"github.com/qcasey/MDroid-Core/hooks"
	"github.com/qcasey/MDroid-Core/mqtt"
	"github.com/qcasey/viper"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/gorilla/mux"
)

// Data points to an underlying viper instance
var Data *viper.Viper

// HL holds the registered hooks for settings
var HL *hooks.HookList

func init() {
	HL = new(hooks.HookList)
}

// ParseConfig will take initial configuration values and parse them into global settings
func ParseConfig(settingsFile string) {
	Data = viper.New()
	Data.SetConfigName(settingsFile) // name of config file (without extension)
	Data.AddConfigPath(".")          // optionally look for config in the working directory
	err := Data.ReadInConfig()       // Find and read the config file
	if err != nil {
		log.Warn().Msg(err.Error())
	}
	Data.WatchConfig()

	// Enable debugging from settings
	if Data.IsSet("mdroid.debug") && Data.GetBool("mdroid.debug") {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	// Check if MQTT has an address and will be setup
	flushToMQTT := Data.IsSet("mdroid.mqtt_address")
	if flushToMQTT {
		log.Info().Msg("Setting up MQTT")
		// Set up MQTT, more dependent than other packages
		if !Data.IsSet("mdroid.MQTT_ADDRESS") || !Data.IsSet("mdroid.MQTT_ADDRESS_FALLBACK") || !Data.IsSet("mdroid.MQTT_CLIENT_ID") || !Data.IsSet("mdroid.MQTT_USERNAME") || !Data.IsSet("mdroid.MQTT_PASSWORD") {
			log.Warn().Msgf("Missing MQTT setup variables, skipping MQTT.")
		} else {
			mqtt.Setup(Data.GetString("mdroid.MQTT_ADDRESS"), Data.GetString("mdroid.MQTT_ADDRESS_FALLBACK"), Data.GetString("mdroid.MQTT_CLIENT_ID"), Data.GetString("mdroid.MQTT_USERNAME"), Data.GetString("mdroid.MQTT_PASSWORD"))
		}
	}

	// Run hooks on all new settings
	settings := Data.AllKeys()
	log.Info().Msg("Settings:")
	for _, key := range settings {
		log.Info().Msgf("\t%s = %s", key, Data.GetString(key))
		if flushToMQTT {
			log.Info().Msg("\t\t- Flushing to MQTT")
			go mqtt.Publish(fmt.Sprintf("settings/%s", key), Data.GetString(key), true)
		}
		HL.RunHooks(key)
	}
}

// HandleGetAll returns all current settings
func HandleGetAll(w http.ResponseWriter, r *http.Request) {
	log.Debug().Msg("Responding to GET request with entire settings map.")
	resp := response.JSONResponse{Output: Data.AllSettings(), Status: "success", OK: true}
	resp.Write(&w, r)
}

// HandleGet returns all the values of a specific setting
func HandleGet(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	componentName := format.Name(params["key"])

	log.Debug().Msgf("Responding to GET request for setting component %s", componentName)

	resp := response.JSONResponse{Output: Data.Get(params["key"]), OK: true}
	if !Data.IsSet(params["key"]) {
		resp = response.JSONResponse{Output: "Setting not found.", OK: false}
	}

	resp.Write(&w, r)
}

// HandleSet is the http wrapper for our setting setter
func HandleSet(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)

	// Parse out params
	key := params["key"]
	value := params["value"]

	// Log if requested
	log.Debug().Msgf("Responding to POST request for setting %s to be value %s", key, value)

	// Do the dirty work elsewhere
	Set(key, value)

	// Respond with OK
	response := response.JSONResponse{Output: key, OK: true}
	response.Write(&w, r)
}

// Set will handle actually updates or posts a new setting value
func Set(key string, value interface{}) error {
	Data.Set(key, value)

	// Post to MQTT
	topic := fmt.Sprintf("settings/%s", key)
	go mqtt.Publish(strings.ToLower(topic), value, true)

	// Log our success
	log.Info().Msgf("Updated setting of %s to %s", key, value)

	Data.WriteConfig()

	// Trigger hooks
	HL.RunHooks(key)

	return nil
}

// Get will check if the given key exists, if not it will create it with the provided value
func Get(key string, value interface{}) interface{} {
	if !Data.IsSet(key) {
		Set(key, value)
	}
	return Data.Get(key)
}
