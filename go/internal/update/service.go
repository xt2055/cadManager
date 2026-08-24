package update

import "strconv"

type Manifest struct {
	Version     string `json:"version"`
	Notes       string `json:"notes"`
	PublishedAt string `json:"publishedAt,omitempty"`
	DownloadURL string `json:"downloadUrl,omitempty"`
	Mandatory   bool   `json:"mandatory"`
}

type CheckResult struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
	Notes           string `json:"notes"`
	PublishedAt     string `json:"publishedAt,omitempty"`
	DownloadURL     string `json:"downloadUrl,omitempty"`
	Mandatory       bool   `json:"mandatory"`
	Platform        string `json:"platform,omitempty"`
}

func Check(manifest Manifest, currentVersion string, platform string) CheckResult {
	if currentVersion == "" {
		currentVersion = manifest.Version
	}
	return CheckResult{
		CurrentVersion:  currentVersion,
		LatestVersion:   manifest.Version,
		UpdateAvailable: CompareVersions(manifest.Version, currentVersion) > 0,
		Notes:           manifest.Notes,
		PublishedAt:     manifest.PublishedAt,
		DownloadURL:     manifest.DownloadURL,
		Mandatory:       manifest.Mandatory,
		Platform:        platform,
	}
}

func CompareVersions(left string, right string) int {
	leftVersion := parseVersion(left)
	rightVersion := parseVersion(right)
	for index := range leftVersion.numbers {
		if leftVersion.numbers[index] > rightVersion.numbers[index] {
			return 1
		}
		if leftVersion.numbers[index] < rightVersion.numbers[index] {
			return -1
		}
	}

	if leftVersion.preRelease == rightVersion.preRelease {
		return 0
	}
	if leftVersion.preRelease == "" {
		return 1
	}
	if rightVersion.preRelease == "" {
		return -1
	}
	if leftVersion.preRelease > rightVersion.preRelease {
		return 1
	}
	return -1
}

type parsedVersion struct {
	numbers    [3]int
	preRelease string
}

func parseVersion(value string) parsedVersion {
	value = trimVersionPrefix(value)
	parts := splitVersionSuffix(value)
	result := parsedVersion{}
	for index, number := range splitByDot(parts[0]) {
		if index >= len(result.numbers) {
			break
		}
		result.numbers[index] = parseInt(number)
	}
	result.preRelease = parts[1]
	return result
}

func trimVersionPrefix(value string) string {
	for len(value) > 0 && (value[0] == 'v' || value[0] == 'V' || value[0] == ' ') {
		value = value[1:]
	}
	return value
}

func splitVersionSuffix(value string) [2]string {
	for index, character := range value {
		if character == '-' || character == '+' {
			return [2]string{value[:index], value[index+1:]}
		}
	}
	return [2]string{value, ""}
}

func splitByDot(value string) []string {
	result := []string{""}
	for _, character := range value {
		if character == '.' {
			result = append(result, "")
			continue
		}
		result[len(result)-1] += string(character)
	}
	return result
}

func parseInt(value string) int {
	result, err := strconv.Atoi(value)
	if err != nil || result < 0 {
		return 0
	}
	return result
}
