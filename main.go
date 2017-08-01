package main

import (
	"github.com/spark404/go-coremidi"
	"fmt"
	"bytes"
	"net/http"
	"time"
	"encoding/json"
	"io/ioutil"
	"log"
)

type Playback struct {
	Group string
	Page int
	Index int
	TitanId int
	Active bool
}

func main() {
	client, err := coremidi.NewClient("midithing")

	if err != nil {
		fmt.Println(err)
		return
	}

	devices, err := coremidi.AllDevices()

	if (err != nil) {
		fmt.Println(err)
		return
	}

	launchpads := Filter(devices, func (v coremidi.Device) bool {
		return "Launchpad Pro" == v.Name()
	})

	if len(launchpads) == 0 {
		fmt.Println("Device \"Launchpad Pro\" not found")
		return
	}

	launchpad := launchpads[0] // Use the first one regardless
	fmt.Printf("Found %v : %v\n", launchpad.Manufacturer(), launchpad.Name())

	if launchpad.IsOffline() {
		fmt.Printf("Device is not online\n");

		return
	}

	entities, err := launchpad.Entities()

	if (err != nil) {
		fmt.Println(err)
		return
	}

	entities = FindEntityByName(entities, "Standalone Port");
	if len(entities) == 0 {
		fmt.Println("Failed to find port")
	}

	entity := entities[0]

	destinations, err := entity.Destinations()

	if (err != nil) {
		fmt.Println(err)
		return
	}

	destination := destinations[0];
	
	fmt.Printf("Connecting to %s using destination %s\n", launchpad.Name(), destination.Name())

	outPort, err := coremidi.NewOutputPort(client, "Midithing Out")
	
	if (err != nil) {
		fmt.Println(err)
		return
	}
	
	var modeChange bytes.Buffer
	modeChange.WriteByte(0xF0)
	modeChange.WriteByte(0x00)
	modeChange.WriteByte(0x20)
	modeChange.WriteByte(0x29)
	modeChange.WriteByte(0x02)
	modeChange.WriteByte(0x10)
	modeChange.WriteByte(0x2C)
	modeChange.WriteByte(0x03) // layout between 0 and 3
	modeChange.WriteByte(0xF7)

	modeChangePacket := coremidi.NewPacket(modeChange.Bytes())
	err = modeChangePacket.Send(&outPort, &destination)
	
	if (err != nil) {
		fmt.Println(err)
		return
	}

	var setAll bytes.Buffer
	setAll.WriteByte(0xF0)
	setAll.WriteByte(0x00)
	setAll.WriteByte(0x20)
	setAll.WriteByte(0x29)
	setAll.WriteByte(0x02)
	setAll.WriteByte(0x10)
	setAll.WriteByte(0x0E) // set all
	setAll.WriteByte(0x00) // color
	setAll.WriteByte(0xF7)

	setAllPacket := coremidi.NewPacket(setAll.Bytes())
	err = setAllPacket.Send(&outPort, &destination)
	
	if (err != nil) {
		fmt.Println(err)
		return
	}


	var netClient = &http.Client{
  		Timeout: time.Second * 10,
	}

	for {

		req, err := http.NewRequest("GET", "http://kubernetes.strocamp.net:8888/playback/", nil)
		req.Header.Add("X-Auth-ApiKey", "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJDb2xvck15U0hBMjA" +
			"xNyIsInN1YiI6Imh1Z29AdHJpcHBhZXJzLm5sIiwibmJmIjoxNDk5NjE2ODI3LCJ" +
			"leHAiOjE1MzExNTI4MjcsImlhdCI6MTQ5OTYxNjgyNywianRpIjoiaWQ0MiIsInR" +
			"5cCI6Imh0dHBzOi8va3ViZXJuZXRlcy5zdHJvY2FtcC5uZXQifQ.vAFytnvw-T-7" +
			"E2OKsbclpki2ZmCwAm_uJq3Q2AgVzj9HBd7_Lw_S1Wtid_MoKMwBWNCEN0vne-oq" +
			"HZgJ0krN5rQHNEoOO7BAjaiPKEzyBQ6l6iWvuavimrpWML0g1Cj2npwZbbcclAHN" +
			"nCtwDQLKWQnQgGlR1qtEB3M4pzTkqJEerqC4ZrQXdKx3qchyDoRN4D6lbsuX1N5j" +
			"gAuqiULcVwF_0y8No_HkpURWWzPY0wPVN7iOi6PAJwIerA-adue6N-zqlIyxNkNo" +
			"A5ybjaAw01BU5cMPv3Yi_0EqeyDY8Etk4y8kMjKsBdRLPom2smiDpNwYinIoy5qN" +
			"hiuArq1szdKPwK9IfQ9ByxcMC3mgOadv0nLkViAEEsBRQLDpoyKo6uCaEiaG2Lwh" +
			"y8VXY2K1XOMlmWKPGwv4DMKR6hmum8e9gCyX_xiWzR1CHMy-Ey632-7-A_2MDxKn" +
			"UF5KzygmN35L1N6OsYUHARlM0Mcw4gD1v85lJz7AvanMxDx5YAUYhSDsB1KJTaQ-" +
			"WT5RTmhOLdrBdIDE9SHmZPoUFCTgsmX4NLSeab8yYLm3j4OMj2coA49-C8RgPprp" +
			"69ClNsKnNHKfXSKqMMemGRwAg4_JlGlgHOXQP8CzdTk-HDc8kn-C-xdWfakPsWvG" +
			"DyT_rtrNxFCs4ZNrT6zRvIg38WyGZNs")
		req.Header.Add("Accept", "application/json")

		resp, err := netClient.Do(req)
		if (err != nil) {
			fmt.Println(err)
			return
		}

		if resp.Status != "200 " {
			log.Fatal("Response code " + resp.Status + " received")
		}
		rawJson, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Fatal(err)
		}

		var playbacks []Playback
		err = json.Unmarshal(rawJson, &playbacks)
		if err != nil {
			fmt.Println("error:", err)
		}
		
		for _, element := range playbacks {
			var ledOn bytes.Buffer
			ledOn.WriteByte(0x90)
			ledOn.WriteByte(mapIndexToLaunchpadButton(element.Index))
			color := 0x2D
			if element.TitanId != 0 {
				color = 72
			}
			ledOn.WriteByte(byte(color))

			ledOnPacket := coremidi.NewPacket(ledOn.Bytes())
			err = ledOnPacket.Send(&outPort, &destination)
			if (err != nil) {
				fmt.Println(err)
			}
		}

		time.Sleep(10 * time.Second)
	}
}

func Filter(vs []coremidi.Device, f func(coremidi.Device) bool) []coremidi.Device {
    vsf := make([]coremidi.Device, 0)
    for _, v := range vs {
        if f(v) {
            vsf = append(vsf, v)
        }
    }
    return vsf
}

func FindEntityByName(entities []coremidi.Entity, needle string) []coremidi.Entity {
	vsf := make([]coremidi.Entity, 0)
	for _, element := range entities {
		if element.Name() == needle {
			vsf = append(vsf, element)
		}
	}

	return vsf
}

func mapIndexToLaunchpadButton(index int) byte {
	if (index < 0 || index > 63) {
		log.Fatal("Index " + string(index) + " doesn't fit on the display")
	}
	return byte((8 - index / 8) * 10 + index % 8 + 1)
	// (8 − TRUNC(+ $A2 ÷ 8) ) × 10 + MOD(A2;8) + 1
}