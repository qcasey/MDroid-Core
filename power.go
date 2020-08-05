package main

import (
	"fmt"
	"time"

	"github.com/qcasey/MDroid-Core/mserial"
	"github.com/qcasey/MDroid-Core/sessions"
	"github.com/qcasey/MDroid-Core/settings"
	"github.com/rs/zerolog/log"
)

func isKeyIn() bool {
	return sessions.Data.GetString("KEY_STATE") != "FALSE"
}

// Evaluates if the doors should be locked
func evalAutoLock() {
	accOn := sessions.Data.GetString("ACC_POWER") == "TRUE"
	isHome := sessions.Data.GetString("BLE_CENTRAL_CONNECTED") == "TRUE"
	isKeyIn := isKeyIn()

	if !sessions.Data.IsSet("DOORS_LOCKED") {
		// Don't log, likely just doesn't exist in session yet
		return
	}

	target := settings.Get("mdroid.autolock", "auto")
	shouldBeOn := sessions.Data.GetString("DOORS_LOCKED") == "FALSE" && !accOn && !isHome && !isKeyIn

	// Instead of power trigger, evaluate here. Lock once every so often
	if target == "AUTO" && shouldBeOn {
		lockToggleTime, err := time.Parse("", sessions.Data.GetString("DOORS_LOCKED.lastUpdate"))
		if err != nil {
			log.Error().Msg(err.Error())
			return
		}

		// For debugging
		log.Info().Msg(lockToggleTime.String())
		//lockedInLast15Mins := time.Since(_lock.lastCheck.time) < time.Minute*15
		unlockedInLast5Minutes := time.Since(lockToggleTime) < time.Minute*5 // handle case where car is UNLOCKED recently, i.e. getting back in. Before putting key in

		if unlockedInLast5Minutes {
			return
		}

		//_lock.lastCheck = triggerType{time: time.Now(), target: _lock.target}
		err = mserial.AwaitText("toggleDoorLocks")
		if err != nil {
			log.Error().Msg(err.Error())
		}
	}
}

// Evaluates if the board should be put to sleep
func evalAutoSleep() {
	accOn := sessions.Data.GetString("ACC_POWER") == "TRUE"
	isHome := sessions.Data.GetString("BLE_CENTRAL_CONNECTED") == "TRUE"
	sleepEnabled := settings.Data.GetString("MDROID.AUTO_SLEEP")

	// If "OFF", auto sleep is not enabled. Exit
	if sleepEnabled != "ON" {
		return
	}

	// Don't fall asleep if the board was recently started
	if time.Since(sessions.GetStartTime()) < time.Minute*10 {
		return
	}

	// Sleep indefinitely, hand power control to the arduino
	if !accOn && isHome && !isKeyIn() {
		sleepMDroid()
	}
}

// Evaluates if the angel eyes should be on, and then passes that struct along as generic power module
func evalAngelEyesPower() {
	isKeyIn := isKeyIn()
	lightSensor := sessions.Data.GetString("LIGHT_SENSOR_ON") == "TRUE"

	shouldBeOn := !lightSensor && isKeyIn
	triggerReason := fmt.Sprintf("lightSensor: %t, keyIsIn: %t", lightSensor, isKeyIn)

	// Pass angel module to generic power trigger
	powerTrigger(shouldBeOn, triggerReason, "ANGEL_EYES")
}

// Evaluates if the cameras and tablet should be on, and then passes that struct along as generic power module
func evalLowPowerMode() {
	accOn := sessions.Data.GetString("ACC_POWER") == "TRUE"
	isHome := sessions.Data.GetString("BLE_CENTRAL_CONNECTED") == "TRUE"
	isKeyIn := isKeyIn()
	startedRecently := time.Since(sessions.GetStartTime()) < time.Minute*5

	shouldBeOn := (accOn && !isHome && !startedRecently) || ((isHome || startedRecently) && isKeyIn)
	triggerReason := fmt.Sprintf("accOn: %t, isHome: %t, keyIsIn: %t, startedRecently: %t", accOn, isHome, isKeyIn, startedRecently)

	// Pass angel module to generic power trigger
	powerTrigger(shouldBeOn, triggerReason, "USB_HUB")
}

// Error check against module's status fetches, then check if we're powering on or off
func powerTrigger(shouldBeOn bool, reason string, componentName string) {
	moduleIsOn := sessions.Data.GetString(fmt.Sprintf("%s_POWER", componentName)) == "TRUE"
	moduleSetting := settings.Data.GetString(fmt.Sprintf("%s.power", componentName))

	// Add a limit to how many checks can occur
	/*
		if module.powerStats.lastTrigger.target != module.target && time.Since(module.powerStats.lastTrigger.time) < time.Second*3 {
			log.Info().Msgf("Ignoring target %s on module %s, since last check was under 3 seconds ago", name, module.target)
			return
		}*/

	var triggerType string
	// Evaluate power target with trigger and settings info
	if (moduleSetting == "AUTO" && !moduleIsOn && shouldBeOn) || (moduleSetting == "ON" && !moduleIsOn) {
		message := mserial.Message{Device: mserial.Writer, Text: fmt.Sprintf("powerOn:%s", componentName)}
		triggerType = "on"
		mserial.Await(&message)
	} else if (moduleSetting == "AUTO" && moduleIsOn && !shouldBeOn) || (moduleSetting == "OFF" && moduleIsOn) {
		triggerType = "off"
		mserial.AwaitText(fmt.Sprintf("powerOff:%s", componentName))
	} else {
		return
	}

	// Log and set next time threshold
	if moduleSetting != "AUTO" {
		reason = fmt.Sprintf("target is %s", moduleSetting)
	}
	log.Info().Msgf("Powering %s %s, because %s", triggerType, componentName, reason)

	//module.powerStats.lastTrigger = powerTrigger{time: time.Now(), target: moduleSetting}
}
