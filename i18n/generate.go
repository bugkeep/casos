// Copyright 2023 The Casos Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package i18n

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/casosorg/casos/util"
)

type I18nData map[string]map[string]string

var (
	reI18nFrontendCall      *regexp.Regexp
	reI18nFrontendProperty  *regexp.Regexp
	reI18nFrontendKey       *regexp.Regexp
	reI18nBackendObject     *regexp.Regexp
	reI18nBackendController *regexp.Regexp
)

func init() {
	// A frontend key reaches i18next in one of two shapes, and both have to be
	// matched here: a key the extractor misses is a key that never lands in the
	// locale files, and i18next then falls back to rendering the key itself, so
	// English looks correct and only the other language is visibly broken.
	//
	// The first shape is an argument to a translate call, written either as
	// i18next.t(...) or as the t(...) that useTranslation returns. Capturing the
	// whole argument list rather than just a leading string is what covers
	// t("key", {count: n}) and t(cond ? "a" : "b"). Requiring a word boundary
	// before the t keeps split(...) and its kin out.
	//
	// The second shape is a key held in an object and translated later, which is
	// how nav.js carries the sidebar and breadcrumb labels and how
	// helmCompatibilityErrors.js maps an error code to a message. Nothing about
	// "word:word" tells a key apart from a Tailwind class or a URL, so those are
	// kept honest by the namespace filter in parseAllWords rather than by the
	// pattern.
	reI18nFrontendCall, _ = regexp.Compile(`\bt\(((?:[^()"]|"[^"]*")*)\)`)
	reI18nFrontendProperty, _ = regexp.Compile(`[A-Za-z_$][\w$]*:\s*("[A-Za-z][A-Za-z0-9]*:[^"\s][^"]*")`)
	reI18nFrontendKey, _ = regexp.Compile(`"([A-Za-z][A-Za-z0-9]*:[^"\s][^"]*)"`)
	reI18nBackendObject, _ = regexp.Compile("i18n.Translate\\((.*?)\"\\)")
	reI18nBackendController, _ = regexp.Compile("c.T\\((.*?)\"\\)")
}

func getAllI18nStringsFrontend(fileContent string) []string {
	return matchI18nKeys(reI18nFrontendCall, fileContent)
}

func getAllI18nPropertyStringsFrontend(fileContent string) []string {
	return matchI18nKeys(reI18nFrontendProperty, fileContent)
}

func matchI18nKeys(re *regexp.Regexp, fileContent string) []string {
	res := []string{}
	for _, match := range re.FindAllStringSubmatch(fileContent, -1) {
		for _, key := range reI18nFrontendKey.FindAllStringSubmatch(match[1], -1) {
			res = append(res, key[1])
		}
	}
	return res
}

func getNamespace(word string) string {
	return strings.SplitN(word, ":", 2)[0]
}

func getAllI18nStringsBackend(fileContent string, isControllerPackage bool) []string {
	res := []string{}
	if isControllerPackage {
		matches := reI18nBackendController.FindAllStringSubmatch(fileContent, -1)
		if matches == nil {
			return res
		}
		for _, match := range matches {
			res = append(res, match[1][1:])
		}
	} else {
		matches := reI18nBackendObject.FindAllStringSubmatch(fileContent, -1)
		if matches == nil {
			return res
		}
		for _, match := range matches {
			match := strings.SplitN(match[1], ",", 2)
			res = append(res, match[1][2:])
		}
	}

	return res
}

func getAllFilePathsInFolder(folder string, fileSuffixes ...string) []string {
	res := []string{}
	err := filepath.Walk(folder,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if strings.HasSuffix(path, "node_modules") {
				return filepath.SkipDir
			}

			if !hasAnySuffix(info.Name(), fileSuffixes) {
				return nil
			}

			res = append(res, path)
			fmt.Println(path, info.Name())
			return nil
		})
	if err != nil {
		panic(err)
	}

	return res
}

func hasAnySuffix(name string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func parseAllWords(category string) *I18nData {
	var paths []string
	if category == "backend" {
		paths = getAllFilePathsInFolder("../", ".go")
	} else {
		// The frontend keeps its components in .jsx and its API clients and
		// helpers in .js, and i18next.t calls appear in both.
		paths = getAllFilePathsInFolder("../web/src", ".js", ".jsx")
	}

	allWords := []string{}
	propertyWords := []string{}
	for _, path := range paths {
		fileContent := util.ReadStringFromPath(path)

		var words []string
		if category == "backend" {
			if strings.HasSuffix(path, "deduplicate_test.go") {
				continue
			}

			isControllerPackage := strings.Contains(path, "controller")
			words = getAllI18nStringsBackend(fileContent, isControllerPackage)
		} else {
			words = getAllI18nStringsFrontend(fileContent)
			propertyWords = append(propertyWords, getAllI18nPropertyStringsFrontend(fileContent)...)
		}
		allWords = append(allWords, words...)
	}

	// A translate call is unambiguous, so the namespaces it names are the whole
	// set the frontend has. Object properties are not: "sm:max-w-lg" and
	// "https://charts.rancher.io" have the same shape as a key, and only the
	// namespace tells them apart.
	namespaces := map[string]bool{}
	for _, word := range allWords {
		namespaces[getNamespace(word)] = true
	}
	for _, word := range propertyWords {
		if namespaces[getNamespace(word)] {
			allWords = append(allWords, word)
		}
	}

	fmt.Printf("%v\n", allWords)

	data := I18nData{}
	for _, word := range allWords {
		tokens := strings.SplitN(word, ":", 2)
		namespace := tokens[0]
		key := tokens[1]

		if _, ok := data[namespace]; !ok {
			data[namespace] = map[string]string{}
		}
		data[namespace][key] = key
	}

	return &data
}

func copyI18nData(src *I18nData) *I18nData {
	dst := I18nData{}
	for namespace, pairs := range *src {
		dst[namespace] = make(map[string]string)
		for key, value := range pairs {
			dst[namespace][key] = value
		}
	}
	return &dst
}

func applyToOtherLanguage(category string, language string, newData *I18nData) {
	oldData := readI18nFile(category, language)
	println(oldData)

	dataCopy := copyI18nData(newData)
	applyData(dataCopy, oldData)
	writeI18nFile(category, language, dataCopy)
}
