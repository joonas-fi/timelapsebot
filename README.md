About
-----

24/7 recording timelapse bot in Golang, intended for Raspberry Pi.

This program is intended to be crash safe in that the only state that it uses is
read from filesystem when the program is re-started.

This program also uses concurrency to try to ensure that the stills are taken at as
fixed interval as possible - any background processing is done concurrently.


Running
-------

```
$ go build timelapsebot.go
$ nohup ./timelapsebot &
```

TODO: systemd unit file


Background processes
--------------------

- Every 5 minutes, encodes all stills (currently ~60 stills / 5 mins) into a single video.
- Every hour, combine those 5-min clips (12 x 5min = hour) into an hour-long video.
- Every 24 hours, combine those hour-long clips (24 x hour = day) into a daily video.
- When a new daily video is produced, upload that into AWS S3 and delete the video
  from SD card so disk use does not grow unbounded.


Install gstreamer
-----------------

Gstreamer is required to take advantage of Raspberry's hardware encoding of h264 and decoding of JPEGs.

Otherwise, we could use something else like mencoder or libav.

With software encoding the first-gen Raspberry Pi (that I use for timelapses) would not be able to keep up.

TODO:

- Currently, the hardware encoded quality is abysmal. Find out how to fix it?
- Use ffmpeg? Does it work on Pi 1? https://ubuntu-mate.community/t/hardware-h264-video-encoding-with-libav-openmax-il/4997

From here https://www.raspberrypi.org/forums/viewtopic.php?t=72435

```
$ sudo sh -c 'echo deb http://vontaene.de/raspbian-updates/ . main >> /etc/apt/sources.list'
$ sudo apt-get install libgstreamer1.0-0 liborc-0.4-0 gir1.2-gst-plugins-base-1.0 gir1.2-gstreamer-1.0 gstreamer1.0-alsa gstreamer1.0-omx gstreamer1.0-plugins-bad gstreamer1.0-plugins-base gstreamer1.0-plugins-base-apps gstreamer1.0-plugins-good gstreamer1.0-plugins-ugly gstreamer1.0-pulseaudio gstreamer1.0-tools gstreamer1.0-x libgstreamer-plugins-bad1.0-0 libgstreamer-plugins-base1.0-0
```

Install avimerge
----------------

```
$ apt-get install -y transcode
$ avimerge -v
avimerge (transcode v1.1.7) (C) 2001-2004 Thomas Oestreich, T. Bitterberg 2004-2010 Transcode Team
```


Optional LED support
--------------------

LED is turned on for duration of still capture, so you can see the "heartbeat" and know that the rig is healthy.

LED support:

```
$ git clone git://git.drogon.net/wiringPi && cd wiringPi && ./build

# Test LED

$ gpio -g mode 16 output
$ gpio -g write 16 1 # off
$ gpio -g write 16 0 # on

# configure LED not to trigger on SD card activity, but GPIO only
$ echo gpio >/sys/class/leds/led0/trigger

# set GPIO #16 (the LED) as output mode
$ gpio -g mode 16 output
```

