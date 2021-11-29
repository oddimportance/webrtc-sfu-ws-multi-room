package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"text/template"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

var (
	addr     = flag.String("addr", ":7676", "http service address")
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	indexTemplate = &template.Template{}

	// lock for peerConnections and trackLocals
	listLock        sync.RWMutex
	// peerConnections []peerConnectionState
	// make peerConnections a map of string
	// basically peerconnections binding to the
	// given room id
	peerConnections map[string][]peerConnectionState
	// trackLocals must be adjusted accordingly
	// but will deal with it later on...
	trackLocals     map[string]*webrtc.TrackLocalStaticRTP

	// @ gorilla/mux
	// create the router
	muxRouting	= mux.NewRouter()

	roomId	string
)

type websocketMessage struct {
	Event string `json:"event"`
	Data  string `json:"data"`
}

type peerConnectionState struct {
	peerConnection *webrtc.PeerConnection
	websocket      *threadSafeWriter
}

func main() {
	// Parse the flags passed to program
	flag.Parse()

	// Init other state
	log.SetFlags(0)
	trackLocals = map[string]*webrtc.TrackLocalStaticRTP{}

	// Read index.html from disk into memory, serve whenever anyone requests /
	indexHTML, err := ioutil.ReadFile("index_ewe.html")
	if err != nil {
		panic(err)
	}

	indexTemplate = template.Must(template.New("").Parse(string(indexHTML)))

	http.Handle("/", muxRouting)

	//the base is dashboard
	// index.html handler
	muxRouting.HandleFunc("/", routerDefault)

	// handle the routing
	handleRouting()

	// request a keyframe every 3 seconds
	go func() {
		for range time.NewTicker(time.Second * 3).C {
			dispatchKeyFrame()
		}
	}()

	// start HTTP server
	log.Fatal(http.ListenAndServe(*addr, nil))
}

// the Dashboard
func routerDefault(w http.ResponseWriter, r *http.Request) {
	var muxVars = mux.Vars(r)

	// do some roomid validation
	// if muxVars["roomid"] == "" || muxVars["roomid"] != "1234" && muxVars["roomid"] != "4321" {

	// 	// Read index.html from disk into memory, serve whenever anyone requests /
	// 	errorHTML, err := ioutil.ReadFile("error_template.html")
	// 	if err != nil {
	// 		panic(err)
	// 	}

	// 	var errorTemplate = template.Must(template.New("").Parse(string(errorHTML)))

	// 	if err := errorTemplate.Execute(w, ""); err != nil {
	// 		log.Fatal(err)
	// 	}

	// }

	roomId = muxVars["roomid"]

	if err := indexTemplate.Execute(w, "wss://"+r.Host+"/websocket"); err != nil {
		log.Fatal(err)
	}

}

func handleRouting() {
	muxRouting.HandleFunc("/{roomid}/websocket", websocketHandler)
	muxRouting.HandleFunc("/websocket", websocketHandler)
	muxRouting.HandleFunc("/{roomid}", routerDefault)
}

// Add to list of tracks and fire renegotation for all PeerConnections
func addTrack(t *webrtc.TrackRemote) *webrtc.TrackLocalStaticRTP {
	listLock.Lock()
	defer func() {
		listLock.Unlock()
		signalPeerConnections()
	}()

	// Create a new TrackLocal with the same codec as our incoming
	trackLocal, err := webrtc.NewTrackLocalStaticRTP(t.Codec().RTPCodecCapability, t.ID(), t.StreamID())
	if err != nil {
		panic(err)
	}

	trackLocals[t.ID()] = trackLocal

	log.Println("Track Added")
	// log.Println(trackLocals)
	return trackLocal
}

// Remove from list of tracks and fire renegotation for all PeerConnections
func removeTrack(t *webrtc.TrackLocalStaticRTP) {
	listLock.Lock()
	defer func() {
		listLock.Unlock()
		signalPeerConnections()
	}()

	delete(trackLocals, t.ID())
}

// signalPeerConnections updates each PeerConnection so that it is getting all the expected media tracks
func signalPeerConnections() {
	listLock.Lock()
	defer func() {
		listLock.Unlock()
		dispatchKeyFrame()
	}()

	log.Println("Attenp Signal Peer")

	attemptSync := func() (tryAgain bool) {
		for i := range peerConnections[roomId] {
			if peerConnections[roomId][i].peerConnection.ConnectionState() == webrtc.PeerConnectionStateClosed {
				peerConnections[roomId] = append(peerConnections[roomId][:i], peerConnections[roomId][i+1:]...)
				return true // We modified the slice, start from the beginning
			}

			// map of sender we already are seanding, so we don't double send
			existingSenders := map[string]bool{}

			for _, sender := range peerConnections[roomId][i].peerConnection.GetSenders() {
				if sender.Track() == nil {
					continue
				}

				existingSenders[sender.Track().ID()] = true

				// If we have a RTPSender that doesn't map to a existing track remove and signal
				if _, ok := trackLocals[sender.Track().ID()]; !ok {
					if err := peerConnections[roomId][i].peerConnection.RemoveTrack(sender); err != nil {
						return true
					}
				}
			}

			// Don't receive videos we are sending, make sure we don't have loopback
			for _, receiver := range peerConnections[roomId][i].peerConnection.GetReceivers() {
				if receiver.Track() == nil {
					continue
				}

				existingSenders[receiver.Track().ID()] = true
			}

			// Add all track we aren't sending yet to the PeerConnection
			for trackID := range trackLocals {
				if _, ok := existingSenders[trackID]; !ok {
					if _, err := peerConnections[roomId][i].peerConnection.AddTrack(trackLocals[trackID]); err != nil {
						return true
					}
				}
			}

			offer, err := peerConnections[roomId][i].peerConnection.CreateOffer(nil)
			if err != nil {
				return true
			}

			if err = peerConnections[roomId][i].peerConnection.SetLocalDescription(offer); err != nil {
				return true
			}

			offerString, err := json.Marshal(offer)
			if err != nil {
				return true
			}

			if err = peerConnections[roomId][i].websocket.WriteJSON(&websocketMessage{
				Event: "offer",
				Data:  string(offerString),
			}); err != nil {
				return true
			}
		}

		return
	}

	for syncAttempt := 0; ; syncAttempt++ {
		if syncAttempt == 25 {
			// Release the lock and attempt a sync in 3 seconds. We might be blocking a RemoveTrack or AddTrack
			go func() {
				time.Sleep(time.Second * 3)
				signalPeerConnections()
			}()
			return
		}

		if !attemptSync() {
			break
		}
	}
}

// dispatchKeyFrame sends a keyframe to all PeerConnections, used everytime a new user joins the call
func dispatchKeyFrame() {
	listLock.Lock()
	defer listLock.Unlock()

	for i := range peerConnections[roomId] {
		for _, receiver := range peerConnections[roomId][i].peerConnection.GetReceivers() {
			if receiver.Track() == nil {
				continue
			}

			_ = peerConnections[roomId][i].peerConnection.WriteRTCP([]rtcp.Packet{
				&rtcp.PictureLossIndication{
					MediaSSRC: uint32(receiver.Track().SSRC()),
				},
			})
		}
	}
}

// Handle incoming websockets
func websocketHandler(w http.ResponseWriter, r *http.Request) {

	var muxVars = mux.Vars(r)

	roomId = muxVars["roomid"]

	// do some roomid validation
	// if muxVars["roomid"] == "" || muxVars["roomid"] != "1234" && muxVars["roomid"] != "4321" {

	// 	// Read index.html from disk into memory, serve whenever anyone requests /
	// 	errorHTML, err := ioutil.ReadFile("error_template.html")
	// 	if err != nil {
	// 		panic(err)
	// 	}

	// 	var errorTemplate = template.Must(template.New("").Parse(string(errorHTML)))

	// 	if err := errorTemplate.Execute(w, ""); err != nil {
	// 		log.Fatal(err)
	// 	}
	// 	log.Println("Invalid Socket ID")

	// 	return

	// }

	// Peer config
	var peerConnectionConfig = webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	// Upgrade HTTP request to Websocket
	unsafeConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}

	c := &threadSafeWriter{unsafeConn, sync.Mutex{}}

	// When this frame returns close the Websocket
	defer c.Close() //nolint

	// Create new PeerConnection
	// peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	peerConnection, err := webrtc.NewPeerConnection(peerConnectionConfig)
	// log.Println(peerConnection)

	if err != nil {
		log.Print(err)
		return
	}

	// When this frame returns close the PeerConnection
	defer peerConnection.Close() //nolint

	// Accept one audio and one video track incoming
	for _, typ := range []webrtc.RTPCodecType{webrtc.RTPCodecTypeVideo, webrtc.RTPCodecTypeAudio} {
		if _, err := peerConnection.AddTransceiverFromKind(typ, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionRecvonly,
		}); err != nil {
			log.Print(err)
			return
		}
	}


	// Add our new PeerConnection to global list
	listLock.Lock()
	// this is where I have manupilated
	// the list of connections
	if peerConnections == nil {
		peerConnections = make(map[string][]peerConnectionState, 0)
	}

	if peerConnections[roomId] == nil {
		peerConnections[roomId] = []peerConnectionState{peerConnectionState{peerConnection, c}}
	}else{
		peerConnections[roomId] = append(peerConnections[roomId], peerConnectionState{peerConnection, c})
	}
	listLock.Unlock()

	// Trickle ICE. Emit server candidate to client
	peerConnection.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			return
		}

		candidateString, err := json.Marshal(i.ToJSON())
		if err != nil {
			log.Println(err)
			return
		}
		// log.Println(string(candidateString))

		if writeErr := c.WriteJSON(&websocketMessage{
			Event: "candidate",
			Data:  string(candidateString),
		}); writeErr != nil {
			log.Println(writeErr)
		}
	})

	// If PeerConnection is closed remove it from global list
	peerConnection.OnConnectionStateChange(func(p webrtc.PeerConnectionState) {
		switch p {
		case webrtc.PeerConnectionStateFailed:
			if err := peerConnection.Close(); err != nil {
				log.Print(err)
			}
		case webrtc.PeerConnectionStateClosed:
			signalPeerConnections()
		}
	})

	peerConnection.OnTrack(func(t *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		// Create a track to fan out our incoming video to all peers
		trackLocal := addTrack(t)
		defer removeTrack(trackLocal)

		buf := make([]byte, 1500)
		for {
			i, _, err := t.Read(buf)
			if err != nil {
				return
			}

			if _, err = trackLocal.Write(buf[:i]); err != nil {
				return
			}
		}
	})

	// Signal for the new PeerConnection
	signalPeerConnections()

	message := &websocketMessage{}
	for {
		_, raw, err := c.ReadMessage()
		if err != nil {
			log.Println(err)
			return
		} else if err := json.Unmarshal(raw, &message); err != nil {
			log.Println(err)
			return
		}

		// log.Println(message)

		switch message.Event {
		case "candidate":
			candidate := webrtc.ICECandidateInit{}
			if err := json.Unmarshal([]byte(message.Data), &candidate); err != nil {
				log.Println(err)
				return
			}

			// log.Println(string(message.Data))

			if err := peerConnection.AddICECandidate(candidate); err != nil {
				log.Println(err)
				return
			}
		case "answer":
			answer := webrtc.SessionDescription{}
			if err := json.Unmarshal([]byte(message.Data), &answer); err != nil {
				log.Println(err)
				return
			}

			if err := peerConnection.SetRemoteDescription(answer); err != nil {
				log.Println(err)
				return
			}
		}
	}
}

// Helper to make Gorilla Websockets threadsafe
type threadSafeWriter struct {
	*websocket.Conn
	sync.Mutex
}

func (t *threadSafeWriter) WriteJSON(v interface{}) error {
	t.Lock()
	defer t.Unlock()

	return t.Conn.WriteJSON(v)
}
