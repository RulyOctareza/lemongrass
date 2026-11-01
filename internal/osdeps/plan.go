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
	s := DepStatus{
		Name:     dep.Name,
		Required: dep.Required,
		Features: dep.Features,
		Floor:    dep.Floor,
		Path:     path,
		Version:  version,
		Origin:   origin,
		Status:   status(dep, path, version),
	}
	needs := fmt.Sprintf("Lemongrass needs %s %s or newer.", dep.Name, dep.Floor)
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
