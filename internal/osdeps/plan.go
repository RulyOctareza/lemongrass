// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package osdeps

import "fmt"

// Statuses of a dependency.
const (
	StatusOK             = "ok"
	StatusMissing        = "missing"
	StatusBelowFloor     = "belowFloor"
	StatusUnknownVersion = "unknownVersion"
)

// Offer actions and routes.
const (
	ActionInstall = "install"
	ActionUpgrade = "upgrade"
	// lgrassroot run through pkexec, on the Linux families.
	RouteHelper = "helper"
	// winget run by the app as the user; an installer that needs admin raises its own UAC prompt.
	RouteWinget = "winget"
)

// Offer is the one-click install or upgrade the app shows for a dependency.
type Offer struct {
	Action  string `json:"action"`
	Package string `json:"package"`
	Route   string `json:"route"`
}

// DepStatus is one dependency's line in the check's output.
type DepStatus struct {
	Name     string   `json:"name"`
	Required bool     `json:"required"`
	Features []string `json:"features"`
	Floor    string   `json:"floor"`
	Path     string   `json:"path"`
	Version  string   `json:"version"`
	Origin   Origin   `json:"origin"`
	Status   string   `json:"status"`
	Offer    *Offer   `json:"offer"`
	Message  string   `json:"message"`
}

func newStatus(dep Dep, path, version string, origin Origin) DepStatus {
	return DepStatus{
		Name:     dep.Name,
		Required: dep.Required,
		Features: dep.Features,
		Floor:    dep.Floor,
		Path:     path,
		Version:  version,
		Origin:   origin,
		Status:   status(dep, path, version),
	}
}

func needsText(dep Dep) string {
	return fmt.Sprintf("Lemongrass needs %s %s or newer.", dep.Name, dep.Floor)
}

func status(dep Dep, path, version string) string {
	switch {
	case path == "":
		return StatusMissing
	case dep.Floor == "":
		return StatusOK
	case version == "":
		return StatusUnknownVersion
	case CompareVersions(version, dep.Floor) < 0:
		return StatusBelowFloor
	}
	return StatusOK
}

// evaluate decides a dependency's status, offer and message from what the check found. A binary installed by hand is
// never updated, since lemongrass updates the user's own installation only through the OS's package manager.
func evaluate(dep Dep, info OSInfo, path, version string, origin Origin) DepStatus {
	if info.Family == FamilyWindows {
		return evaluateWindows(dep, info, path, version, origin)
	}
	s := newStatus(dep, path, version, origin)
	needs := needsText(dep)
	switch s.Status {
	case StatusMissing:
		pkg := dep.Packages[info.Family]
		switch {
		case dep.CheckOnly || pkg == "":
			s.Message = dep.NoRoute[info.Family]
		case info.Atomic:
			s.Message = fmt.Sprintf("This system adds tools by layering them onto its image. Run rpm-ostree install %s and restart.", pkg)
		case info.Family == FamilyFedora && HelperAllows(info.Family, pkg):
			s.Offer = &Offer{Action: ActionInstall, Package: pkg, Route: RouteHelper}
		}
	case StatusBelowFloor:
		switch {
		case origin.Kind == OriginManual:
			s.Message = fmt.Sprintf("The %s at %s was installed by hand, so lemongrass does not update it. %s", dep.Name, path, needs)
		case info.Atomic:
			s.Message = fmt.Sprintf("%s comes with this system's image and updates with system updates. %s", dep.Name, needs)
		case origin.Kind == OriginPackage && info.Family == FamilyFedora && !dep.CheckOnly &&
			HelperAllows(info.Family, origin.Package):
			s.Offer = &Offer{Action: ActionUpgrade, Package: origin.Package, Route: RouteHelper}
		default:
			s.Message = needs
		}
	case StatusUnknownVersion:
		s.Message = fmt.Sprintf("Lemongrass could not read the version of the %s at %s. %s", dep.Name, path, needs)
	}
	return s
}

// evaluateWindows offers winget for a missing tool and for one winget can upgrade: a package it unpacked, or Git for
// Windows, which winget upgrades whoever installed it. Scoop's copies are left to Scoop.
func evaluateWindows(dep Dep, info OSInfo, path, version string, origin Origin) DepStatus {
	s := newStatus(dep, path, version, origin)
	needs := needsText(dep)
	noWinget := fmt.Sprintf("winget is not available, so lemongrass cannot install %s. winget comes with App Installer from the Microsoft Store.", dep.Name)
	switch s.Status {
	case StatusMissing:
		pkg := dep.Packages[FamilyWindows]
		switch {
		case dep.CheckOnly || pkg == "":
			s.Message = dep.NoRoute[FamilyWindows]
		case !info.Winget:
			s.Message = noWinget
		default:
			s.Offer = &Offer{Action: ActionInstall, Package: pkg, Route: RouteWinget}
		}
	case StatusBelowFloor:
		switch {
		case dep.CheckOnly:
			s.Message = needs
		case origin.Manager == ManagerScoop:
			s.Message = fmt.Sprintf("The %s at %s comes from Scoop, so lemongrass does not update it. Run scoop update %s. %s", dep.Name, path, origin.Package, needs)
		case origin.Manager == ManagerWinget || origin.Manager == ManagerInstaller:
			if info.Winget {
				s.Offer = &Offer{Action: ActionUpgrade, Package: origin.Package, Route: RouteWinget}
			} else {
				s.Message = noWinget + " " + needs
			}
		case origin.Kind == OriginManual:
			s.Message = fmt.Sprintf("The %s at %s was installed by hand, so lemongrass does not update it. %s", dep.Name, path, needs)
		default:
			s.Message = needs
		}
	case StatusUnknownVersion:
		s.Message = fmt.Sprintf("Lemongrass could not read the version of the %s at %s. %s", dep.Name, path, needs)
	}
	return s
}

// blockedText explains a Windows tool that would not start, which on a machine with Smart App Control is what an
// unsigned program looks like.
func blockedText(dep Dep) string {
	return fmt.Sprintf("Windows did not let %s run. Smart App Control blocks programs that are not signed, and %s is not.", dep.Name, dep.Name)
}
