package main

import (
	"github.com/qcasey/MDroid-Core/sessions"
)

//
// We're taking actions based on the values or a combination of values
// from the session/settings.
//

// When angel eyes setting is changed
func angelEyesSettings(key string, value interface{}) {
	// Determine state of angel eyes
	go evalAngelEyesPower()
}

// When auto lock setting is changed
func autoLockSettings(key string, value interface{}) {
	// Trigger state of auto lock
	go evalAutoLock()
}

// When auto Sleep setting is changed
func autoSleepSettings(key string, value interface{}) {
	// Trigger state of auto sleep
	go evalAutoSleep()
}

// When ACC power state is changed
func accPower() {
	// Trigger low power and auto sleep
	go evalAngelEyesPower()
	go evalBluetoothDeviceState()
	go evalLowPowerMode()
	go evalAutoLock()
	go evalAutoSleep()
}

// When light sensor is changed in session
func lightSensorOn() {
	// Determine state of angel eyes
	go evalAngelEyesPower()
}

// Alert me when it's raining and windows are down
func lightSensorReason() {
	keyPosition := sessions.Data.GetString("key_position.value")
	windowsOpen := sessions.Data.GetString("windows_open.value")
	doorsLocked := sessions.Data.GetString("doors_locked.value")

	if sessions.Data.GetString("light_sensor_reason.value") == "RAIN" &&
		keyPosition == "OFF" &&
		doorsLocked == "TRUE" &&
		windowsOpen == "TRUE" {
		sessions.SlackAlert("Windows are down in the rain, eh?")
	}
}

func seatMemory() {
	sendServiceCommand("MDROID", "restart")
}
