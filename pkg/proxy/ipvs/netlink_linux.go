// +build linux

/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ipvs

import (
	"fmt"
	"github.com/vishvananda/netlink"
	"k8s.io/apimachinery/pkg/util/sets"
	"net"
	"syscall"
)

type netlinkHandle struct {
	netlink.Handle
}

// NewNetLinkHandle will crate a new netlinkHandle
func NewNetLinkHandle() NetLinkHandle {
	return &netlinkHandle{netlink.Handle{}}
}

// EnsureAddressBind checks if address is bound to the interface and, if not, binds it. If the address is already bound, return true.
func (h *netlinkHandle) EnsureAddressBind(address, devName string) (exist bool, err error) {
	dev, err := h.LinkByName(devName)
	if err != nil {
		return false, fmt.Errorf("error get interface: %s, err: %v", devName, err)
	}
	addr := net.ParseIP(address)
	if addr == nil {
		return false, fmt.Errorf("error parse ip address: %s", address)
	}
	if err := h.AddrAdd(dev, &netlink.Addr{IPNet: netlink.NewIPNet(addr)}); err != nil {
		// "EEXIST" will be returned if the address is already bound to device
		if err == syscall.Errno(syscall.EEXIST) {
			return true, nil
		}
		return false, fmt.Errorf("error bind address: %s to interface: %s, err: %v", address, devName, err)
	}
	return false, nil
}

// UnbindAddress unbind address from the interface
func (h *netlinkHandle) UnbindAddress(address, devName string) error {
	dev, err := h.LinkByName(devName)
	if err != nil {
		return fmt.Errorf("error get interface: %s, err: %v", devName, err)
	}
	addr := net.ParseIP(address)
	if addr == nil {
		return fmt.Errorf("error parse ip address: %s", address)
	}
	if err := h.AddrDel(dev, &netlink.Addr{IPNet: netlink.NewIPNet(addr)}); err != nil {
		return fmt.Errorf("error unbind address: %s from interface: %s, err: %v", address, devName, err)
	}
	return nil
}

// EnsureDummyDevice is part of interface
func (h *netlinkHandle) EnsureDummyDevice(devName string) (bool, error) {
	_, err := h.LinkByName(devName)
	if err == nil {
		// found dummy device
		return true, nil
	}
	dummy := &netlink.Dummy{
		LinkAttrs: netlink.LinkAttrs{Name: devName},
	}
	return false, h.LinkAdd(dummy)
}

// DeleteDummyDevice is part of interface.
func (h *netlinkHandle) DeleteDummyDevice(devName string) error {
	link, err := h.LinkByName(devName)
	if err != nil {
		return fmt.Errorf("error deleting a non-exist dummy device: %s", devName)
	}
	dummy, ok := link.(*netlink.Dummy)
	if !ok {
		return fmt.Errorf("expect dummy device, got device type: %s", link.Type())
	}
	return h.LinkDel(dummy)
}
func (h *netlinkHandle) GetLocalAddresses(filterDev string) (sets.String, error) {
	linkIndex := -1
	if len(filterDev) != 0 {
		link, err := h.LinkByName(filterDev)
		if err != nil {
			return nil, fmt.Errorf("error get filter device %s, err: %v", filterDev, err)
		}
		linkIndex = link.Attrs().Index
	}

	routeFilter := &netlink.Route{
		Table:    syscall.RT_TABLE_LOCAL,
		Type:     syscall.RTN_LOCAL,
		Protocol: syscall.RTPROT_KERNEL,
	}
	filterMask := netlink.RT_FILTER_TABLE | netlink.RT_FILTER_TYPE | netlink.RT_FILTER_PROTOCOL

	// find filter device
	if linkIndex != -1 {
		routeFilter.LinkIndex = linkIndex
		filterMask |= netlink.RT_FILTER_OIF
	}

	routes, err := h.RouteListFiltered(netlink.FAMILY_ALL, routeFilter, filterMask)
	if err != nil {
		return nil, fmt.Errorf("error list route table, err: %v", err)
	}
	res := sets.NewString()
	for _, route := range routes {
		if route.Src != nil {
			res.Insert(route.Src.String())
		}
	}
	return res, nil
}
