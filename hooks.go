package main

import (
	"fmt"
	"math"
	"strconv"

	"github.com/MrDoctorKovacic/MDroid-Core/formatting"
	"github.com/MrDoctorKovacic/MDroid-Core/gps"
	"github.com/MrDoctorKovacic/MDroid-Core/sessions"
	"github.com/rs/zerolog/log"

	"github.com/MrDoctorKovacic/MDroid-Core/mserial"
	"github.com/MrDoctorKovacic/MDroid-Core/settings"
)

// Define temporary holding struct for power values
type power struct {
	on          bool
	powerTarget string
	errOn       error
	errTarget   error
	triggerOn   bool
	settingComp string
	settingName string
}

// Read the target action based on current ACC Power value
var (
	soundDef    = power{settingComp: "SOUND", settingName: "POWER"}
	wirelessDef = power{settingComp: "WIRELESS", settingName: "POWER"}
	angelDef    = power{settingComp: "ANGEL_EYES", settingName: "POWER"}
	tabletDef   = power{settingComp: "TABLET", settingName: "POWER"}
	boardDef    = power{settingComp: "BOARD", settingName: "POWER"}
)

func setupHooks() {
	settings.RegisterHook("ANGEL_EYES", angelEyesSettings)
	settings.RegisterHook("WIRELESS", wirelessSettings)
	sessions.RegisterHookSlice(&[]string{"MAIN_VOLTAGE_RAW", "AUX_VOLTAGE_RAW"}, voltage)
	sessions.RegisterHook("AUX_CURRENT_RAW", auxCurrent)
	sessions.RegisterHook("ACC_POWER", accPower)
	sessions.RegisterHook("KEY_STATE", keyState)
	sessions.RegisterHook("WIRELESS_POWER", wirelessPower)
	sessions.RegisterHook("LIGHT_SENSOR_REASON", lightSensorReason)
	sessions.RegisterHook("LIGHT_SENSOR_ON", lightSensorOn)
	sessions.RegisterHookSlice(&[]string{"SEAT_MEMORY_1", "SEAT_MEMORY_2", "SEAT_MEMORY_3"}, voltage)
}

// Helper function to generalize fetching session string
func getSessionString(name string, def string) string {
	v, err := sessions.Get(name)
	if err != nil {
		log.Debug().Msg(fmt.Sprintf("%s could not be determined, defaulting to FALSE", name))
		v.Value = def
	}
	return v.Value
}

// Helper function to generalize fetching session bool
func getSessionBool(name string, def bool) bool {
	v, err := sessions.GetBool(name)
	if err != nil {
		log.Debug().Msg(fmt.Sprintf("%s could not be determined, defaulting to false", name))
		v = def
	}
	return v
}

//
// From here on out are the hook functions.
// We're taking actions based on the values or a combination of values
// from the session/settings post values.
//

// When angel eyes setting is changed
func angelEyesSettings(settingName string, settingValue string) {
	// Determine state of angel eyes
	evalAngelEyes(getSessionString("KEY_STATE", "FALSE"))
}

// When key state is changed in session
func keyState(hook *sessions.SessionPackage) {
	// Determine state of angel eyes
	evalAngelEyes(hook.Data.Value)

	// Determine state of the video boards
	evalVideo(hook.Data.Value)
}

// When light sensor is changed in session
func lightSensorOn(hook *sessions.SessionPackage) {
	// Determine state of angel eyes
	evalAngelEyes(getSessionString("KEY_STATE", "FALSE"))
}

// Evaluates if the angel eyes should be on, and then passes that struct along as generic power module
func evalAngelEyes(keyIsIn string) {
	angel := angelDef
	angel.on, angel.errOn = sessions.GetBool("ANGEL_EYES_POWER")
	angel.powerTarget, angel.errTarget = settings.Get(angel.settingComp, angel.settingName)
	lightSensor := getSessionBool("LIGHT_SENSOR_ON", false)

	shouldTrigger := !lightSensor && keyIsIn != "FALSE"

	// Pass angel module to generic power trigger
	genericPowerTrigger(shouldTrigger, "Angel", angel)
}

// Evaluates if the video boards should be on, and then passes that struct along as generic power module
func evalVideo(keyIsIn string) {
	board := boardDef
	board.on, board.errOn = sessions.GetBool("BOARD_POWER")
	board.powerTarget, board.errTarget = settings.Get(board.settingComp, board.settingName)

	// Pass angel module to generic power trigger
	genericPowerTrigger(keyIsIn != "FALSE", "Board", board)
}

// When wireless setting is changed
func wirelessSettings(settingName string, settingValue string) {
	accOn := getSessionBool("ACC_POWER", false)
	wifiOn := getSessionBool("WIFI_CONNECTED", false)

	// Determine state of wireless
	evalWireless(accOn, wifiOn)
}

// Evaluates if the wireless boards should be on, and then passes that struct along as generic power module
func evalWireless(accOn bool, wifiOn bool) {
	wireless := wirelessDef
	wireless.on, wireless.errOn = sessions.GetBool("WIRELESS_POWER")
	wireless.powerTarget, wireless.errTarget = settings.Get(wireless.settingComp, wireless.settingName)

	// Wireless is most likely supposed to be on, only one case where it should not be
	shouldTrigger := true
	if !accOn && wifiOn {
		shouldTrigger = false
	}

	// Pass angel module to generic power trigger
	genericPowerTrigger(shouldTrigger, "Wireless", wireless)
}

// Evaluates if the sound board should be on, and then passes that struct along as generic power module
func evalSound(accOn bool, wifiOn bool) {
	sound := soundDef
	sound.on, sound.errOn = sessions.GetBool("SOUND_POWER")
	sound.powerTarget, sound.errTarget = settings.Get(sound.settingComp, sound.settingName)

	keyIsIn := getSessionString("KEY_STATE", "FALSE")
	shouldTrigger := accOn && !wifiOn || wifiOn && keyIsIn != "FALSE"

	// Pass angel module to generic power trigger
	genericPowerTrigger(shouldTrigger, "Sound", sound)
}

// Convert main raw voltage into an actual number
func voltage(hook *sessions.SessionPackage) {
	voltageFloat, err := strconv.ParseFloat(hook.Data.Value, 64)
	if err != nil {
		log.Error().Msg(fmt.Sprintf("Failed to convert string %s to float", hook.Data.Value))
		return
	}

	sessions.SetValue(hook.Name[0:len(hook.Name)-4], fmt.Sprintf("%.3f", (voltageFloat/1024)*24.4))
}

// Modifiers to the incoming Current sensor value
func auxCurrent(hook *sessions.SessionPackage) {
	currentFloat, err := strconv.ParseFloat(hook.Data.Value, 64)
	if err != nil {
		log.Error().Msg(fmt.Sprintf("Failed to convert string %s to float", hook.Data.Value))
		return
	}

	realCurrent := math.Abs(1000 * ((((currentFloat * 3.3) / 4095.0) - 1.5) / 185))
	sessions.SetValue("AUX_CURRENT", fmt.Sprintf("%.3f", realCurrent))
}

// Trigger for booting boards/tablets
func accPower(hook *sessions.SessionPackage) {
	// Read the target action based on current ACC Power value
	var accOn bool

	// Check incoming ACC power value is valid
	switch hook.Data.Value {
	case "TRUE":
		accOn = true
	case "FALSE":
		accOn = false
	default:
		log.Error().Msg(fmt.Sprintf("ACC Power Trigger unexpected value: %s", hook.Data.Value))
		return
	}

	// Pull all the necessary configuration data
	tablet := tabletDef
	tablet.on, tablet.errOn = sessions.GetBool("TABLET_POWER")
	tablet.powerTarget, tablet.errTarget = settings.Get(tablet.settingComp, tablet.settingName)
	wifiOn := getSessionBool("WIFI_CONNECTED", false)

	// Trigger wireless, based on ACC and wifi status
	go evalWireless(accOn, wifiOn)

	// Trigger wireless, based on ACC and wifi status
	go evalSound(accOn, wifiOn)

	// Trigger tablet, based on ACC status
	go genericPowerTrigger(accOn, "Tablet", tablet)
}

// Error check against module's status fetches, then check if we're powering on or off
func genericPowerTrigger(shouldBeOn bool, name string, module power) {
	if module.errOn == nil && module.errTarget == nil {
		if (module.powerTarget == "AUTO" && !module.on && shouldBeOn) || (module.powerTarget == "ON" && !module.on) {
			log.Info().Msg(fmt.Sprintf("Powering on %s, because target is %s", name, module.powerTarget))
			mserial.Push(settings.Config.SerialControlDevice, fmt.Sprintf("powerOn%s", name))
		} else if (module.powerTarget == "AUTO" && module.on && !shouldBeOn) || (module.powerTarget == "OFF" && module.on) {
			log.Info().Msg(fmt.Sprintf("Powering off %s, because target is %s", name, module.powerTarget))
			gracefulShutdown(name)
		}
	} else if module.errTarget != nil {
		log.Error().Msg(fmt.Sprintf("Setting Error: %s", module.errTarget.Error()))
		if module.settingComp != "" && module.settingName != "" {
			log.Error().Msg(fmt.Sprintf("Setting read error for %s. Resetting to AUTO", name))
			settings.Set(module.settingComp, module.settingName, "AUTO")
		}
	} else if module.errOn != nil {
		log.Debug().Msg(fmt.Sprintf("Session Error: %s", module.errOn.Error()))
	}
}

// When wireless is turned off, we can infer that LTE is also off
func wirelessPower(hook *sessions.SessionPackage) {
	if hook.Data.Value == "FALSE" {
		// When board is turned off but doesn't have time to reflect LTE status
		sessions.SetValue("LTE_ON", "FALSE")
	}
}

// Alert me when it's raining and windows are down
func lightSensorReason(hook *sessions.SessionPackage) {
	keyPosition, _ := sessions.Get("KEY_POSITION")
	doorsLocked, _ := sessions.Get("DOORS_LOCKED")
	windowsOpen, _ := sessions.Get("WINDOWS_OPEN")
	delta, err := formatting.CompareTimeToNow(doorsLocked.LastUpdate, gps.GetTimezone())

	if err != nil {
		if hook.Data.Value == "RAIN" &&
			keyPosition.Value == "OFF" &&
			doorsLocked.Value == "TRUE" &&
			windowsOpen.Value == "TRUE" &&
			delta.Minutes() > 5 {
			sessions.SlackAlert(settings.Config.SlackURL, "Windows are down in the rain, eh?")
		}
	}
}

// Restart different machines when seat memory buttons are pressed
func seatMemory(hook *sessions.SessionPackage) {
	switch hook.Name {
	case "SEAT_MEMORY_1":
		mserial.CommandNetworkMachine("BOARD", "restart")
	case "SEAT_MEMORY_2":
		mserial.CommandNetworkMachine("WIRELESS", "restart")
	case "SEAT_MEMORY_3":
		mserial.CommandNetworkMachine("MDROID", "restart")
	}
}
