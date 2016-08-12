package main

import (
	"fmt"
    "strings"
    "regexp"
    "io/ioutil"
    "encoding/json"

	"github.com/docker/libnetwork/iptables"
	"github.com/vishvananda/netlink"
	log "github.com/Sirupsen/logrus"
)

const (
    jsonDb = "/var/run/docker/emc_fip_db.json"
	preRoutingChainName = "FLOAT_IP_PREROUTING"
	postRoutingChainName = "FLOAT_IP_POSTROUTING"
)

//fipStr must end with /24 or /\d\d
func bind(ifaceName string, fipStr string, lipStr string) error {
	setInterfaceIP(ifaceName, fipStr)
	fipStr = clearIPMask(fipStr)
	lipStr = clearIPMask(lipStr)
	dnat := []string{
		preRoutingChainName, "-t", "nat",
		"-d", fipStr,
		"-j", "DNAT",
		 "--to-destination", lipStr,
	}
	if _, err := iptables.Raw(
		append([]string{"-C"}, dnat...)...,
	); err != nil {
		incl := append([]string{"-I"}, dnat...)
		if _, err = iptables.Raw(incl...); err != nil {
			log.Fatalf("Can not add rule: %s",strings.Join(incl, " ") )
		}
	}
	log.Debugf("Add a DNAT rule to iptables %s chain: %s to %s ", preRoutingChainName, fipStr, lipStr)

	snat := []string{
		postRoutingChainName, "-t", "nat",
		"-s", lipStr,
		"-j", "SNAT",
		"--to-source", fipStr,
	}
	if _, err := iptables.Raw(
		append([]string{"-C"}, snat...)...,
	); err != nil {
		incl := append([]string{"-I"}, snat...)
		if _, err = iptables.Raw(incl...); err != nil {
			log.Fatalf("Can not add rule %s",strings.Join(incl, " ") )
		}
	}
	log.Debugf("Add a SNAT rule to iptables %s chain: %s to %s ", postRoutingChainName, lipStr, fipStr)
	return nil
}


//fipStr must end with /24 or /\d\d
func unbind(ifaceName string, fipStr string, lipStr string) error {
	delInterfaceIP(ifaceName, fipStr)
	fipStr = clearIPMask(fipStr)
	lipStr = clearIPMask(lipStr)
	dnat := []string{
		preRoutingChainName, "-t", "nat",
		"-d", fipStr,
		"-j", "DNAT", "--to-destination",  lipStr,
	}
	iptables.Raw(
		append([]string{"-D"}, dnat...)...,
	); 
	log.Debugf("DELETE a DNAT rule to iptables %s chain: %s to %s ", preRoutingChainName,fipStr, lipStr)
	
	snat := []string{
		postRoutingChainName, "-t", "nat",
		"-s", lipStr,
		"-j", "SNAT", "--to-source",  fipStr,
	}
	iptables.Raw(
		append([]string{"-D"}, snat...)...,
	)
	log.Debugf("DELETE a SNAT rule to iptables %s chain: %s to %s ", postRoutingChainName, lipStr, fipStr)	
	return nil
}

func initIptables() {
	
	if _, err := iptables.NewChain(preRoutingChainName, "nat", false); err != nil {
		log.Fatalf("IPtables Maintainer: can not created %s", preRoutingChainName)
	}
	if _, err := iptables.NewChain(postRoutingChainName, "nat", false); err != nil {
		log.Fatalf("IPtables Maintainer: can not created %s", postRoutingChainName)
	}

    addPre := []string{
		"-C", "PREROUTING",
        "-j", preRoutingChainName,
		"-t", "nat",
		"-m", "addrtype",
		"--dst-type", "LOCAL" ,
	}
    if _, err := iptables.Raw(addPre...); err != nil {
		addPre[0] = "-A"
		if _, err = iptables.Raw(addPre...); err != nil {
			log.Fatalf("Can not add chain %s",strings.Join(addPre, " ") )
		}
	}

    addPost := []string{
		"-C", "POSTROUTING",
		"-j", postRoutingChainName,
		"-t", "nat",
		"-m", "addrtype",
		"--dst-type", "LOCAL" ,
	}
   	if _, err := iptables.Raw(addPost...); err != nil {
		addPost[0] = "-A" 
		if _, err = iptables.Raw(addPost...); err != nil {
			log.Fatalf("Can not add chain %s",strings.Join(addPost, " ") )
		}
	}

	addOutput := []string{
		"-C", "OUTPUT",
		"-j", preRoutingChainName,
		"-t", "nat",
		"-m", "addrtype",
		"--dst-type", "LOCAL" ,
	}
   	if _, err := iptables.Raw(addOutput...); err != nil {
		addOutput[0] = "-A"
		if _, err = iptables.Raw(addOutput...); err != nil {
			log.Fatalf("Can not add rule %s",strings.Join(addOutput, " ") )
		}
	}
}

func getDNATList() []map[string]string {
    dnatList:= make([]map[string]string, 0, 100)
    dockerChain := []string{
		"-nL", preRoutingChainName,
		"-t", "nat",
	}
    res, err := iptables.Raw(dockerChain...)
    if err != nil {
        fmt.Print(err)
    }
    pattern, _ := regexp.CompilePOSIX("^DNAT.*$")
    re := pattern.FindAll(res, -1)
    for i:=0; i <= len(re) - 1 ; i++ {
        fields := strings.Fields(string(re[i]))
		fipStr := clearIPMask(fields[4])
		lipStr := clearIPMask(strings.Split(fields[5], ":")[1])
		dnatList = append(dnatList, map[string]string {"floating_ip": fipStr, "managed_ip": lipStr})
    }
    return dnatList
}

func getSNATList() []map[string]string {
    snatList := make([]map[string]string, 0, 100)
    dockerChain := []string{
		"-nL", postRoutingChainName,
		"-t", "nat",
	}
    res, err := iptables.Raw(dockerChain...)
    if err != nil {
        fmt.Print(err)
    }
    pattern, _ := regexp.CompilePOSIX("^SNAT.*$")
    re := pattern.FindAll(res, -1)
    for i:=0; i <= len(re) - 1 ; i++ {
		fields := strings.Fields(string(re[i]))
		fipStr := strings.Split(fields[5], ":")[1]
		lipStr := fields[3]
        snatList = append(snatList, map[string]string {"floating_ip": fipStr, "managed_ip": lipStr})
    }
    return snatList
}

func iptablesMaintainer(linkName string, firstAddr netlink.Addr) {
	
    var network map[string]map[string]string
    raw, err := ioutil.ReadFile(jsonDb)
    if err != nil {
       log.Debug("JsonDb is not exist, load fail")
    } else {
		json.Unmarshal(raw, &network)
	}
	
    ips := make([]map[string]string, 0, 100)
    for _, n := range network {
		ip := map[string]string{"floating_ip": n["floating_ip"], "managed_ip": n["managed_ip"]}
        ips = append(ips, ip)
    }

	// maintain DNAT rule
	dnatList := getDNATList();
    for _, dnat := range dnatList {
		//log.Debugf("DNAT Rule String: %s", dnat)
        if !eleInListMapArr(dnat, ips) {
            unbind(linkName, dnat["floating_ip"], dnat["managed_ip"])
	        log.Debugf("IPtables Maintainer: Del ip for interface [ %s ] : %s,  And remove iptables rules", linkName, dnat["floating_ip"])
        } 
    }

	for _, ip := range ips {
		 if !eleInListMapArr(ip, dnatList) {
            bind(linkName, ip["floating_ip"], ip["managed_ip"])
	        log.Debugf("IPtables Maintainer: Add ip for interface [ %s ] : %s,  And remove iptables rules", linkName, ip["floating_ip"])
        } 
	}

	// maintain DNAT rule
	snatList :=  getSNATList()
    for _, snat := range snatList{
		//log.Debugf("SNAT Rule String: %s", snat)
        if !eleInListMapArr(snat, ips) {
            unbind(linkName, snat["floating_ip"], snat["managed_ip"])
	        log.Debugf("IPtables Maintainer: Del ip for interface [ %s ]: %s, And remove iptables rules", linkName, snat["floating_ip"])
        }
    }

	for _, ip := range ips {
		 if !eleInListMapArr(ip, snatList) {
            bind(linkName, ip["floating_ip"], ip["managed_ip"])
	        log.Debugf("IPtables Maintainer: Add ip for interface [ %s ] : %s,  And remove iptables rules", linkName, ip["floating_ip"])
        } 
	}

	// maintain ip addr
	log.Debugf("Keep ip clean for [ %s ]", linkName)
	iface := getLinkByName(linkName)
	retries := 3

	err = nil
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

	err = nil
	for _, addr := range addrs {
		if addr.String() == firstAddr.String()  {
			continue
		}
		isAddrInMetaDataList := false
		for _, ip := range ips {
			fields := strings.Fields(addr.String())
			if fields[0] == ip["floating_ip"] {
				isAddrInMetaDataList = true
				break
			}
		}
		if !isAddrInMetaDataList {
			for i := 1; i <= retries; i++ {
				err = netlink.AddrDel(iface, &addr)
				log.Infof("Del IP: [ %s ]", addr.String())
				if err == nil {
					break
				}
			}	
			if err != nil {
				log.Fatalf("Can not get ip list of %s", linkName)
			}
		}
	}
}