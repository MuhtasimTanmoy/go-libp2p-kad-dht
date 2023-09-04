package coord

import (
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/plprobelab/go-kademlia/kad"
	"github.com/plprobelab/go-kademlia/key"

	"github.com/libp2p/go-libp2p-kad-dht/v2/kadt"
)

// NodeInfoToAddrInfo converts a kad.NodeInfo to a peer.AddrInfo.
// This function will panic if info.ID() does not return a kadt.PeerID
func NodeInfoToAddrInfo(info kad.NodeInfo[key.Key256, ma.Multiaddr]) peer.AddrInfo {
	peerID := info.ID().(kadt.PeerID)
	return peer.AddrInfo{
		ID:    peer.ID(peerID),
		Addrs: info.Addresses(),
	}
}

// NodeIDToAddrInfo converts a kad.NodeID to a peer.AddrInfo with no addresses.
// This function will panic if id's underlying type is not kadt.PeerID
func NodeIDToAddrInfo(id kad.NodeID[key.Key256]) peer.AddrInfo {
	peerID := id.(kadt.PeerID)
	return peer.AddrInfo{
		ID: peer.ID(peerID),
	}
}

// SliceOfNodeInfoToSliceOfAddrInfo converts a kad.NodeInfo to a peer.AddrInfo.
// This function will panic if any info.ID() does not return a kadt.PeerID
func SliceOfNodeInfoToSliceOfAddrInfo(infos []kad.NodeInfo[key.Key256, ma.Multiaddr]) []peer.AddrInfo {
	peers := make([]peer.AddrInfo, len(infos))
	for i := range infos {
		peerID := infos[i].ID().(kadt.PeerID)
		peers[i] = peer.AddrInfo{
			ID:    peer.ID(peerID),
			Addrs: infos[i].Addresses(),
		}
	}
	return peers
}

// SliceOfPeerIDToSliceOfNodeID converts a slice peer.ID to a slice of kad.NodeID
func SliceOfPeerIDToSliceOfNodeID(peers []peer.ID) []kad.NodeID[key.Key256] {
	nodes := make([]kad.NodeID[key.Key256], len(peers))
	for i := range peers {
		nodes[i] = kadt.PeerID(peers[i])
	}
	return nodes
}

// NodeIDToPeerID converts a kad.NodeID to a peer.ID.
// This function will panic if id's underlying type is not kadt.PeerID
func NodeIDToPeerID(id kad.NodeID[key.Key256]) peer.ID {
	return peer.ID(id.(kadt.PeerID))
}
