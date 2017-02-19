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

/*
	LED support
	-----------

	Set LED to trigger on GPIO:

	$ sudo su

	Install WiringPi

	$ git clone git://git.drogon.net/wiringPi && cd wiringPi && ./build

	Test LED

	$ gpio -g mode 16 output
	$ gpio -g write 16 1 # off
	$ gpio -g write 16 0 # on


	To have avimerge
	----------------

	$ apt-get install -y transcode
	$ avimerge -v
	avimerge (transcode v1.1.7) (C) 2001-2004 Thomas Oestreich, T. Bitterberg 2004-2010 Transcode Team


*/
// Install 
//
// $ sudo apt-get install libav-tools
//
// https://www.raspberrypi.org/forums/viewtopic.php?t=72435
// 	$ sudo sh -c 'echo deb http://vontaene.de/raspbian-updates/ . main >> /etc/apt/sources.list'
// 	$ sudo apt-get install libgstreamer1.0-0 liborc-0.4-0 gir1.2-gst-plugins-base-1.0 gir1.2-gstreamer-1.0 gstreamer1.0-alsa gstreamer1.0-omx gstreamer1.0-plugins-bad gstreamer1.0-plugins-base gstreamer1.0-plugins-base-apps gstreamer1.0-plugins-good gstreamer1.0-plugins-ugly gstreamer1.0-pulseaudio gstreamer1.0-tools gstreamer1.0-x libgstreamer-plugins-bad1.0-0 libgstreamer-plugins-base1.0-0

/*	This assumes that you've done the following:

	$ echo gpio >/sys/class/leds/led0/trigger
	$ gpio -g mode 16 output
*/
func setRaspberryPowerLed(on bool, state *TimelapseState) {
	if !state.ledSupported {
		return
	}

	bitStatus := "0" // for on (SIC)

	if !on {
		bitStatus = "1" // for off
	}

	exec.Command("gpio", "-g", "write", "16", bitStatus).Start()

	// log.Printf("setRaspberryPowerLed: bit = %s (0 => on, 1 => off)", bitStatus)
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

/*
func tickEverySecond(tickChan chan int) {
	for {
		now := time.Now()
		nextSec := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second() + 1, 0, time.UTC)

		time.Sleep(nextSec.Sub(now))

		// tickChan <- nextSec.String()
		tickChan <- TICK_1SEC
	}
}
*/

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

/*
$ tree timelapse/
timelapse/
___ bucket_5min
___ bucket_day
___ bucket_hour
___ bucket_minute
___ ___ 1.jpg
___ ___ 2.jpg
___ ___ 3.jpg


*/

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

func stillsTo5min(state *TimelapseState) {
	fiveMinVideoFilename := state.headFromState.Format("2006-01-02_03-04.avi")

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

	// avconv -f image2 -i /home/pi/timelapse/bucket_stills_temp/%d.jpg -r 12 -s 1920x1440 /home/pi/timelapse/bucket_5min/foo.mkv
	// args := []string{"avconv", "-f", "image2", "-i", state.dirStillsTemp + "/%d.jpg", "-r", "12", "-s", "1920x1440", outFile}

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

func fiveMinsToHour(state *TimelapseState) {
	hourVideoMerged := state.dirHour + "/" + state.headFromState.Format("2006-01-02_03.avi")

	log.Printf("fiveMinsToHour: %s -> %s", state.dir5min, hourVideoMerged)

	// TODO: make goroutines
	mergeVideosInternal(state.contents5mins, hourVideoMerged)

	state.contents5mins = []string{}
	state.contentsHours = append(state.contentsHours, hourVideoMerged)
}

func hoursToDay(state *TimelapseState) {
	dayVideoMerged := state.dirDaily + "/" + state.headFromState.Format("2006-01-02.avi")

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
		"-w", "1280",
		"-h", "960",
		"-o", state.dirStills + "/" + strconv.Itoa(stillNumber) + ".jpg",
	}

	// log.Printf("takeStill: invoking %s", strings.Join(args, " "))
	log.Printf("takeStill: starting")

	argv := args[1:]
	cmd := exec.Command(args[0], argv...)

	setRaspberryPowerLed(true, state)

	if output, err := cmd.CombinedOutput(); err != nil {		
		fmt.Fprintln(os.Stderr, string(output), err)
		panic(err)
	}

	setRaspberryPowerLed(false, state)
}

func main() {
	state := restoreState()

	setRaspberryPowerLed(false, &state)

	for {
		now := time.Now()
		// in 5 seconds (TODO: this will drift a bit)
		nextTickShouldBe := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second() + 5, 0, time.UTC)
		nowAccordingToInternet := timeToLast5Mins(now)

		differentDay := nowAccordingToInternet.Day() != state.headFromState.Day()
		differentHour := differentDay || nowAccordingToInternet.Hour() != state.headFromState.Hour()
		different5Min := !nowAccordingToInternet.Equal(state.headFromState)

		// TODO: have these run concurrently WRT takeStil(), so these periodical chores do not delay takeStill()
		if different5Min {
			stillsTo5minBootstrap(&state)

			runRestConcurrently := func () {
				stillsTo5min(&state)

				// different5Min is always true along with differentHour || differentDay
				
				if differentHour {
					// depends on result of stillsTo5min()
					fiveMinsToHour(&state)
				}

				if differentDay {
					// depends on result of fiveMinsToHour()
					hoursToDay(&state)
				}
			}

			go runRestConcurrently()
		}

		// should run right after stillsTo5min() bootstrap (dir rename, new dir) is done
		takeStill(&state)

		state.headFromState = nowAccordingToInternet

		durationToNextTick := nextTickShouldBe.Sub(time.Now())

		if (durationToNextTick > 0) { // can be negative (if we're late) or 0
			time.Sleep(durationToNextTick)
		}
	}
}
