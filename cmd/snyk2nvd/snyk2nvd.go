// Copyright (c) Facebook, Inc. and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/facebookincubator/nvdtools/providers/lib/runner"
	"github.com/facebookincubator/nvdtools/providers/snyk/api"
	"github.com/facebookincubator/nvdtools/providers/snyk/schema"
)

const (
	baseURL   = "https://data.snyk.io/api/v4"
	userAgent = "snyk2nvd"
)

var lf languageFilter

func Read(r io.Reader, c chan runner.Convertible) error {
	var vulns map[string]*schema.Advisory
	if err := json.NewDecoder(r).Decode(&vulns); err != nil {
		return fmt.Errorf("can't decode into vulns: %v", err)
	}

	for _, vuln := range vulns {
		if lf.accepts(vuln) {
			c <- vuln
		}
	}

	return nil
}

func FetchSince(baseURL, userAgent string, since int64) (<-chan runner.Convertible, error) {
	consumerID := os.Getenv("SNYK_ID")
	if consumerID == "" {
		return nil, fmt.Errorf("Please set SNYK_ID in environment")
	}
	secret := os.Getenv("SNYK_READONLY_KEY")
	if secret == "" {
		return nil, fmt.Errorf("Please set SNYK_READONLY_KEY in environment")
	}

	client := api.NewClient(baseURL, userAgent, consumerID, secret)

	advs, err := client.FetchAllVulnerabilities(since)
	return lf.filter(advs), err
}

func main() {
	flag.Var(&lf, "language", "Comma separated list of languages to download/convert. If not set, then use all available")
	r := runner.Runner{
		Config: runner.Config{
			BaseURL:   baseURL,
			UserAgent: userAgent,
		},
		FetchSince: FetchSince,
		Read:       Read,
	}

	if err := r.Run(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

// language filter

type languageFilter map[string]bool

// String is a part of flag.Value interface implementation.
func (lf *languageFilter) String() string {
	languages := make([]string, 0, len(*lf))
	for language := range *lf {
		languages = append(languages, language)
	}
	return strings.Join(languages, ",")
}

// Set is a part of flag.Value interface implementation.
func (lf *languageFilter) Set(val string) error {
	if val == "" {
		return nil
	}
	if *lf == nil {
		*lf = make(languageFilter)
	}
	for _, v := range strings.Split(val, ",") {
		if v != "" {
			(*lf)[v] = true
		}
	}
	return nil
}

func (lf *languageFilter) accepts(adv *schema.Advisory) bool {
	return lf == nil || len(*lf) == 0 || (*lf)[adv.Language]
}

func (lf *languageFilter) filter(ch <-chan *schema.Advisory) <-chan runner.Convertible {
	output := make(chan runner.Convertible)
	go func() {
		defer close(output)
		for adv := range ch {
			if lf.accepts(adv) {
				output <- adv
			}
		}
	}()
	return output
}
