# webrtc-sfu-ws-multi-room
This project is an eloboration of pion/webrtc. The idea is to make the (pion/webrtc) [sfu-ws](https://github.com/pion/example-webrtc-applications/tree/master/sfu-ws) example be able to handle multiple rooms.

#### Side note
I've improved the overall look and the usibility a bit by adding remote video carousel. A video, in the remote track carousel, can be clicked to be showen the main projector.

#### Main issue
Although the mother project, [sfu-ws](https://github.com/pion/example-webrtc-applications/tree/master/sfu-ws) works impressively well and is also robost, it missed the multi room feature.

```
Room 1: https://localhost:7676/?room-id=12345

Participants of room 1 = A, B, C, D

Room 2: https://localhost:7676/?room-id=67890

Participants of room 2 = E, F, G, H

```

#### My approach
After some debugging, I have come up with the following "solution". Though I can now isolate peers to a particular room, I'm not sure if this is the correct way of handling multiple rooms with webRTC. In particular, I'm hoping that one of the Pion experts would comment on the eventuell performance issues with this approach.

###### The idea is to bind the list of peer connections to a specific room id:

```
// make list of peer connections globally accessable
// example: peerconnections[1234][..]peerConnectionState{peerConnection, c}
var peerConnections map[string][]peerConnectionState

// Create new PeerConnection
// peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{})
peerConnection, err := webrtc.NewPeerConnection(peerConnectionConfig)


// the list of connections
// initialize connections list
if peerConnections == nil {
    peerConnections = make(map[string][]peerConnectionState, 0)
}

// push peer connection to the list of peer connections of a specific room
if peerConnections[roomId] == nil {
    peerConnections[roomId] = []peerConnectionState{peerConnectionState{peerConnection, c}}
}else{
    peerConnections[roomId] = append(peerConnections[roomId], peerConnectionState{peerConnection, c})
}
```
