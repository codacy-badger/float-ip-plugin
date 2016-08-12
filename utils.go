package main

import (
	"fmt"
	"net"
	"time"
	"os"
	"os/exec"
	"bytes"
    "strings"
	"path/filepath"

	log "github.com/Sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"github.com/docker/libnetwork/iptables"
)


func checkSockFileExist(sockFilePath string) {
	log.Infof("listen to file: %s", sockFilePath)
	if _, err := os.Stat(sockFilePath); os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(sockFilePath), 666)
		os.Create(sockFilePath)
	}
}

func mainLink() string {
	route := exec.Command("ip", "route")
	grep :=  exec.Command("grep", "-E", "^default")
	grep.Stdin, _ = route.StdoutPipe()

	var out bytes.Buffer
	grep.Stdout = &out
	_ = grep.Start()
    errRoute := route.Run()
    errGrep := grep.Wait()
    if errRoute != nil || errGrep != nil {
		log.Fatal("Can not get interface")
    }

	name := strings.Fields(out.String())[4]
	log.Debugf("The interface [ %s ] was choosen", name)
    return  name
}

// Check if a netlink interface exists in the default namespace
func validateIface(ifaceStr string) bool {
	_, err := net.InterfaceByName(ifaceStr)
	if err != nil {
		log.Debugf("The requested interface [ %s ] was not found on the host: %s", ifaceStr, err)
		return false
	}
	return true
}

// Set the IP addr of a netlink interface
func setInterfaceIP(name string, rawIP string) error {
	iface := getLinkByName(name)
	ipNet, err := netlink.ParseIPNet(rawIP)
	if err != nil {
		return err
	}
	log.Debugf("Add ip for interface [ %s ] : %s", name, rawIP)
	addr := &netlink.Addr{ipNet, name, 0, 0}
	addrs, _ := netlink.AddrList(iface, netlink.FAMILY_V4)
	if eleInListAddr(*addr, addrs) {
		return nil 
	}
	if err := netlink.AddrAdd(iface, addr); err != nil {
		log.Fatalf("Can not add ip %s for %s", rawIP, name)
	}
	return nil
}

// Delete the IP addr of a netlink interface
func delInterfaceIP(name string, rawIP string) error {
	iface := getLinkByName(name)
	ipNet, err := netlink.ParseIPNet(rawIP)
	if err != nil {
		return err
	}
	log.Debugf("Del ip for interface [ %s ] : %s", name, rawIP)
	addr := &netlink.Addr{ipNet, name, 0, 0}
	addrs, _ := netlink.AddrList(iface, netlink.FAMILY_V4)
	if !eleInListAddr(*addr, addrs) {
		return nil 
	}
	if err := netlink.AddrDel(iface, addr); err != nil {
		log.Fatalf("Can not rm ip %s for %s", rawIP, name)
	}
	return nil
}

//if error then exit program
func getLinkByName(name string) netlink.Link {
	retries := 3
	var iface netlink.Link
	var err error
	for i := 0; i < retries; i++ {
		iface, err = netlink.LinkByName(name)
		if err == nil {
			break
		}
		time.Sleep(2 * time.Second)
		log.Debugf("error retrieving netlink link [ %s ]... retrying", name)
	}
	
	if err != nil {
		log.Fatalf("Abandoning retrieving the link from netlink, Run [ ip link ] to troubleshoot the error: %s", err)
	}
	return iface
}

// Generate a mac addr
func makeMac(ip net.IP) string {
	hw := make(net.HardwareAddr, 6)
	hw[0] = 0x7a
	hw[1] = 0x42
	copy(hw[2:], ip.To4())
	return hw.String()
}

// Return the IPv4 address of a network interface
func getIfaceAddr(name string) (*netlink.Addr, error) {
	iface, err := netlink.LinkByName(name)
	if err != nil {
		return nil, err
	}
	addrs, err := netlink.AddrList(iface, netlink.FAMILY_V4)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("Interface %s has no IP addresses", name)
	}
	if len(addrs) > 1 {
		log.Infof("Interface [ %v ] has more than 1 IPv4 address. Defaulting to using [ %v ]\n", name, addrs[0].IP)
	}
	return &addrs[0], nil
}

// Increment an IP in a subnet
func ipIncrement(networkAddr net.IP) net.IP {
	for i := 15; i >= 0; i-- {
		b := networkAddr[i]
		if b < 255 {
			networkAddr[i] = b + 1
			for xi := i + 1; xi <= 15; xi++ {
				networkAddr[xi] = 0
			}
			break
		}
	}
	return networkAddr
}

// Enable a netlink interface
func interfaceUp(name string) error {
	iface, err := netlink.LinkByName(name)
	if err != nil {
		log.Debugf("Error retrieving a link named [ %s ]", iface.Attrs().Name)
		return err
	}
	return netlink.LinkSetUp(iface)
}

func clearIPMask(rawIP string) string {
	return strings.Split(rawIP, "/")[0]
}

func checkIPWithMask(ips ...string) (string, bool) {
	for _, ip := range ips {
		if ! strings.Contains(ip, "/") {
			return ip, false
		}
	}
	return "", true
}

func eleInListMapArr(needle map[string]string, hystack []map[string]string) bool {
    for _, ele := range hystack {
		if clearIPMask(needle["floating_ip"]) == clearIPMask(ele["floating_ip"]) &&
		clearIPMask(needle["managed_ip"]) == clearIPMask(ele["managed_ip"]) {
			return true
		}
    }
    return false
}

func eleInListAddr(needle netlink.Addr, hystack []netlink.Addr) bool {
    for _, ele := range hystack {
        if needle.String() == ele.String() {
            return true
        }
    }
    return false
}

func clearIPtables(linkName string, firstAddr netlink.Addr) {
	log.Infof("Clean the unused ip for [ %s ]", linkName)
	iface := getLinkByName(linkName)
	retries := 3
	var err error
	var addrs []netlink.Addr
	for i := 1; i <= retries; i++ {
		addrs, err = netlink.AddrList(iface, netlink.FAMILY_V4)
		if err == nil {
			break
		}	
	}
	if err != nil {
		log.Fatalf("Can not get ip list of %s", linkName)
	}
	for _, addr := range addrs {
		if addr.String() != firstAddr.String() {
			if err := netlink.AddrDel(iface, &addr); err != nil {
				log.Fatalf("Can not rm ip %s for %s", firstAddr, linkName)
			}
		}
	}

	log.Infof("Clean the iptables")
	cleanPre := []string{
		"-F", preRoutingChainName,
		"-t", "nat",
	}
	iptables.Raw(cleanPre...)
	cleanPost := []string{
		"-F", postRoutingChainName,
		"-t", "nat",
	}
	iptables.Raw(cleanPost...)
}