package main

import (
	"bytes"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"goipsec/global"
	"goipsec/pkg/config"
	"goipsec/pkg/glog"
	"goipsec/pkg/ipsec"
	"goipsec/pkg/preflight"
	"net"
)

func main() {
	preflight.Checklist()
	config.NewConfig()
	glog.Logger.Print("INFO: starting goipsec")
	listen()
}

func listen() {
	handle, err := pcap.OpenLive("eth0", 65536, true, pcap.BlockForever)
	if err != nil {
		panic(err)
	}

	// sniff traffic
	err = handle.SetBPFFilter("(tcp and src host 2001:db8:23:42:1::10) or esp or (tcp and src host 2001:db8:23:42:1::40)")
	if err != nil {
		panic(err)
	}

	// channels
	send := make(chan gopacket.SerializeBuffer)
	recv := gopacket.NewPacketSource(handle, handle.LinkType()).Packets()

	// convert IP addresses now to avoid re-calling same functions
	clientAddr := net.ParseIP(global.ClientIPv6)
	VPNGatewayAddr := net.ParseIP(global.VPNGatewayIPv6)

	for {
		select {
		case packet := <-recv:
			if packet.Layer(layers.LayerTypeTCP) != nil {
				networkLayer := packet.Layer(layers.LayerTypeIPv6)
				if networkLayer == nil {
					// IPv4 packet
					networkLayer = packet.Layer(layers.LayerTypeIPv4)
					glog.Logger.Printf("INFO: recv IPv4 TCP packet from %s\n", net.IP(networkLayer.LayerContents()[12:16]).String())
				} else {
					// IPv6 packet
					glog.Logger.Printf("INFO: recv IPv6 TCP packet from %s\n", net.IP(networkLayer.LayerContents()[8:24]).String())

					if bytes.Compare(networkLayer.LayerContents()[8:24], clientAddr) == 0 {
						go ipsec.EncryptPacket(packet, send, true)
					} else {
						go ipsec.EncryptPacket(packet, send, false)
					}
				}
			} else if packet.Layer(layers.LayerTypeIPSecESP) != nil {
				IPLayer := packet.Layer(layers.LayerTypeIPv6)
				glog.Logger.Printf("INFO: recv ESP packet from %s\n", net.IP(IPLayer.LayerContents()[8:24]).String())

				if bytes.Compare(IPLayer.LayerContents()[8:24], VPNGatewayAddr) == 0 {
					go ipsec.DecryptPacket(packet, send, true)
				} else {
					go ipsec.DecryptPacket(packet, send, false)
				}
			}
		case packet := <-send:
			err := handle.WritePacketData(packet.Bytes())
			if err != nil {
				glog.Logger.Printf("WARNING: send packet error: %s\n", err)
			}
		}
	}

}
