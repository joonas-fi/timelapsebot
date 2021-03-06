package main

import (
	"time"
	"fmt"
	"os"
	"strings"
	"strconv"
	"os/exec"
	"log"
	"io/ioutil"
)

type TimelapseState struct {
	headFromState time.Time

	ledSupported bool

	// stills

	stillMaxNumber int
	dirStills string
	contentsStills []string
	dirStillsTemp string

	// 5 min

	dir5min string
	contents5mins []string

	// hourly

	dirHour string
	contentsHours []string

	// daily (no need to track contents as we don't merge them)

	dirDaily string
}

func setRaspberryLed(on bool, state *TimelapseState) {
	if !state.ledSupported {
		return
	}

	bitStatus := "0" // for on (SIC)

	if !on {
		bitStatus = "1" // for off
	}

	exec.Command("gpio", "-g", "write", "16", bitStatus).Start()

	// log.Printf("setRaspberryLed: bit = %s (0 => on, 1 => off)", bitStatus)
}

func timeToLast5Mins(ts time.Time) time.Time {
	/*
		10:00:00 -> 10:00
		10:04:59 -> 10:00
		10:05:01 -> 10:05
	*/

	min := ts.Minute()

	return time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), min - (min % 5), 0, 0, time.UTC)
}

func makeDirIfNotExists(dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		log.Printf("makeDirIfNotExists: making %s", dir)

		if err = os.Mkdir(dir, 0755); err != nil {
			panic(err)
		}
	}
}

func restoreState() TimelapseState {
	state := TimelapseState{}
	state.stillMaxNumber = 0
	state.headFromState = time.Time{} // TODO: done automatically?
	state.dirStills = "/home/pi/timelapse/bucket_stills"
	state.dirStillsTemp = "/home/pi/timelapse/bucket_stills_temp"
	state.dir5min = "/home/pi/timelapse/bucket_5min"
	state.dirHour = "/home/pi/timelapse/bucket_hour"
	state.dirDaily = "/home/pi/timelapse/bucket_day"

	// dirStillsTemp left out on purpose because dirStills is renamed to it
	makeDirIfNotExists(state.dirStills)
	makeDirIfNotExists(state.dir5min)
	makeDirIfNotExists(state.dirHour)
	makeDirIfNotExists(state.dirDaily)

	// TODO: detect LED support
	state.ledSupported = true

	// ----------- read stills

	files, err := ioutil.ReadDir(state.dirStills)
	if err != nil {
		panic(err)
	}

	for _, file := range files {
		if state.headFromState.IsZero() {
			state.headFromState = timeToLast5Mins(file.ModTime())
		}
		state.contentsStills = append(state.contentsStills, state.dirStills + "/" + file.Name())

		state.stillMaxNumber++
	}

	// ----------- read 5mins

	files, err = ioutil.ReadDir(state.dir5min)
	if err != nil {
		panic(err)
	}

	for _, file := range files {
		state.contents5mins = append(state.contents5mins, state.dir5min + "/" + file.Name())
	}

	// ----------- read hours

	files, err = ioutil.ReadDir(state.dirHour)
	if err != nil {
		panic(err)
	}

	for _, file := range files {
		state.contentsHours = append(state.contentsHours, state.dirHour + "/" + file.Name())
	}

	// ----------- process the rest

	if state.headFromState.IsZero() {
		state.headFromState = timeToLast5Mins(time.Now())
	}

	return state
}

func stillsTo5minBootstrap(state *TimelapseState) {
	log.Printf("stillsTo5minBootstrap: %s -> %s", state.dirStills, state.dirStillsTemp)

	// /home/pi/timelapse/bucket_stills => /home/pi/timelapse/bucket_stills_temp
	err := os.Rename(state.dirStills, state.dirStillsTemp)
	if err != nil {
		panic(err)
	}

	err = os.Mkdir(state.dirStills, 0755)
	if err != nil {
		panic(err)
	}

	// reset still counter
	state.stillMaxNumber = 0

	// clear all stills
	state.contentsStills = []string{}
}

func stillsTo5min(headFromState time.Time, state *TimelapseState) {
	fiveMinVideoFilename := headFromState.Format("2006-01-02_15-04.avi")

	outFile := state.dir5min + "/" + fiveMinVideoFilename

	state.contents5mins = append(state.contents5mins, outFile)

	// gst-launch-1.0 multifilesrc location=%d.jpg index=1 caps="image/jpeg,framerate=24/1" ! jpegdec ! omxh264enc ! avimux ! filesink location=timelapse.avi
	args := []string{
		"gst-launch-1.0",
		"multifilesrc",
		"location=" + state.dirStillsTemp + "/%d.jpg",
		"index=1",
		"caps=image/jpeg,framerate=24/1",
		"!", "jpegdec",
		"!", "omxh264enc",
		// "!", "omxh264enc", "target-bitrate=800000000",
		"!", "avimux", // use mp4mux?
		"!", "filesink", "location=" + outFile,
	}

	log.Printf("stillsTo5min: invoking %s\n", strings.Join(args, " "))

	argv := args[1:]
	cmd := exec.Command(args[0], argv...)
	if output, err2 := cmd.CombinedOutput(); err2 != nil {
		fmt.Fprintln(os.Stderr, string(output), err2)
		panic(err2)
	}

	log.Printf("stillsTo5min: done, deleting %s\n", state.dirStillsTemp)

	if err3 := os.RemoveAll(state.dirStillsTemp); err3 != nil {
		panic(err3)
	}
}

func mergeVideosInternal(files []string, outFile string) {
	// avimerge -o merged.avi -i in1.avi in2.avi
	args := []string{
		"avimerge",
		"-o", outFile,
	}

	for _, file := range files {
		args = append(args, "-i", file)
	}

	log.Printf("mergeVideosInternal: invoking %s\n", strings.Join(args, " "))

	cmd := exec.Command(args[0], args[1:]...)
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintln(os.Stderr, string(output), err)
		panic(err)
	}

	log.Printf("mergeVideosInternal: done. deleting %d file(s)\n", len(files))

	for _, fileToDelete := range files {
		if deleteErr := os.Remove(fileToDelete); deleteErr != nil {
			panic(deleteErr)
		}
	}
}

func fiveMinsToHour(headFromState time.Time, state *TimelapseState) {
	hourVideoMerged := state.dirHour + "/" + headFromState.Format("2006-01-02_15.avi")

	log.Printf("fiveMinsToHour: %s -> %s", state.dir5min, hourVideoMerged)

	// TODO: make goroutines
	mergeVideosInternal(state.contents5mins, hourVideoMerged)

	state.contents5mins = []string{}
	state.contentsHours = append(state.contentsHours, hourVideoMerged)
}

func hoursToDay(headFromState time.Time, state *TimelapseState) {
	dayVideoMerged := state.dirDaily + "/" + headFromState.Format("2006-01-02.avi")

	log.Printf("hoursToDay: %s -> %s", state.dirHour, dayVideoMerged)

	// TODO: make goroutines
	mergeVideosInternal(state.contentsHours, dayVideoMerged)

	state.contentsHours = []string{}
}

func takeStill(state *TimelapseState) {
	stillNumber := state.stillMaxNumber

	state.stillMaxNumber = state.stillMaxNumber + 1

	args := []string{
		"raspistill",
		"-t", "1000",
		"-w", "1280", // the old Pi doesn't seem capable of encoding higher res h264 streams
		"-h", "960",
		"-o", state.dirStills + "/" + strconv.Itoa(stillNumber) + ".jpg",
	}

	// log.Printf("takeStill: invoking %s", strings.Join(args, " "))
	log.Printf("takeStill: starting")

	argv := args[1:]
	cmd := exec.Command(args[0], argv...)

	setRaspberryLed(true, state)

	if output, err := cmd.CombinedOutput(); err != nil {		
		fmt.Fprintln(os.Stderr, string(output), err)
		panic(err)
	}

	setRaspberryLed(false, state)
}

func main() {
	state := restoreState()

	setRaspberryLed(false, &state)

	nextTick := time.Now()

	for {
		// 16:54:13 => 16:50:00
		tickCeiledTo5Minutes := timeToLast5Mins(nextTick)

		differentDay := tickCeiledTo5Minutes.Day() != state.headFromState.Day()
		differentHour := differentDay || tickCeiledTo5Minutes.Hour() != state.headFromState.Hour()
		different5Min := !tickCeiledTo5Minutes.Equal(state.headFromState)

		// these run concurrently WRT takeStill(), so no lagging should be observed
		if different5Min {
			stillsTo5minBootstrap(&state)

			runRestConcurrently := func (headFromState time.Time) {
				stillsTo5min(headFromState, &state)

				// different5Min is always true along with differentHour || differentDay
				
				if differentHour {
					// depends on result of stillsTo5min()
					fiveMinsToHour(headFromState, &state)
				}

				if differentDay {
					// depends on result of fiveMinsToHour()
					hoursToDay(headFromState, &state)
				}
			}

			go runRestConcurrently(state.headFromState)
		}

		takeStill(&state)

		state.headFromState = tickCeiledTo5Minutes

		nextTick = nextTick.Add(5 * time.Second)
		durationToNextTick := nextTick.Sub(time.Now())

		if (durationToNextTick > 0) { // can be negative (if we're late) or 0
			time.Sleep(durationToNextTick)
		}
	}
}
