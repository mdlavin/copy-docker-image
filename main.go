/*
Copyright 2017 Matt Lavin <matt.lavin@gmail.com>

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

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/alecthomas/kingpin"
	"github.com/docker/distribution/manifest/schema1"
	"github.com/fsouza/go-dockerclient"
	"github.com/heroku/docker-registry-client/registry"
)

func moveLayerUsingFile(srcHub *registry.Registry, destHub *registry.Registry, srcRepo string, destRepo string, layer schema1.FSLayer, file *os.File) error {
	layerDigest := layer.BlobSum

	srcImageReader, err := srcHub.DownloadLayer(srcRepo, layerDigest)
	if err != nil {
		return fmt.Errorf("Failure while starting the download of an image layer. %v", err)
	}

	_, err = io.Copy(file, srcImageReader)
	if err != nil {
		return fmt.Errorf("Failure while copying the image layer to a temp file. %v", err)
	}
	srcImageReader.Close()
	file.Sync()

	imageReadStream, err := os.Open(file.Name())
	if err != nil {
		return fmt.Errorf("Failed to open the temporary image layer for uploading. %v", err)
	}
	err = destHub.UploadLayer(destRepo, layerDigest, imageReadStream)
	imageReadStream.Close()
	if err != nil {
		return fmt.Errorf("Failure while uploading the image. %v", err)
	}

	return nil
}

func migrateLayer(srcHub *registry.Registry, destHub *registry.Registry, srcRepo string, destRepo string, layer schema1.FSLayer) error {
	fmt.Println("Checking if manifest layer exists in destiation registery")

	layerDigest := layer.BlobSum
	hasLayer, err := destHub.HasLayer(destRepo, layerDigest)
	if err != nil {
		return fmt.Errorf("Failure while checking if the destiation registry contained an image layer. %v", err)
	}

	if !hasLayer {
		fmt.Println("Need to upload layer", layerDigest, "to the destiation")
		tempFile, err := ioutil.TempFile("", "docker-image")
		if err != nil {
			return fmt.Errorf("Failure while a creating temporary file for an image layer download. %v", err)
		}

		err = moveLayerUsingFile(srcHub, destHub, srcRepo, destRepo, layer, tempFile)
		removeErr := os.Remove(tempFile.Name())
		if removeErr != nil {
			// Print the error but don't fail the whole migration just because of a leaked temp file
			fmt.Printf("Failed to remove image layer temp file %s. %v", tempFile.Name(), removeErr)
		}

		return err
	}

	fmt.Println("Layer already exists in the destination")
	return nil

}

type RepositoryArguments struct {
	RegistryURL *string
	Repository  *string
	Tag         *string
}

func buildRegistryArguments(argPrefix string, argDescription string) RepositoryArguments {
	registryURLName := fmt.Sprintf("%sURL", argPrefix)
	registryURLDescription := fmt.Sprintf("URL of %s registry", argDescription)
	registryURLArg := kingpin.Flag(registryURLName, registryURLDescription).String()

	repositoryName := fmt.Sprintf("%sRepo", argPrefix)
	repositoryDescription := fmt.Sprintf("Name of the %s repository", argDescription)
	repositoryArg := kingpin.Flag(repositoryName, repositoryDescription).String()

	tagName := fmt.Sprintf("%sTag", argPrefix)
	tagDescription := fmt.Sprintf("Name of the %s tag", argDescription)
	tagArg := kingpin.Flag(tagName, tagDescription).String()

	return RepositoryArguments{
		RegistryURL: registryURLArg,
		Repository:  repositoryArg,
		Tag:         tagArg,
	}
}

func connectToRegistry(args RepositoryArguments, auths map[string]docker.AuthConfiguration) (*registry.Registry, error) {

	url := *args.RegistryURL
	urlWithoutPrefix := strings.TrimPrefix(url, "https://")

	username := ""
	password := ""

	if auth, ok := auths[urlWithoutPrefix]; ok {
		username = auth.Username
		password = auth.Password
	}

	registry, err := registry.NewInsecure(url, username, password)
	if err != nil {
		return nil, fmt.Errorf("Failed to create registry connection for %s. %v", url, err)
	}

	err = registry.Ping()
	if err != nil {
		return nil, fmt.Errorf("Failed to to ping registry %s as a connection test. %v", url, err)
	}

	return registry, nil
}

func main() {
	exitCode := 0
	defer func() {
		os.Exit(exitCode)
	}()

	auths, err := docker.NewAuthConfigurationsFromDockerCfg()
	if err != nil {
		fmt.Printf("Couldn't read config.json in .docker folder\n")
	}

	srcArgs := buildRegistryArguments("src", "source")
	destArgs := buildRegistryArguments("dest", "destiation")
	repoArg := kingpin.Flag("repo", "The repository in the source and the destiation. Values provided by --srcRepo or --destTag will override this value").String()
	tagArg := kingpin.Flag("tag", "The tag name in the source and the destiation. Values provided by --srcTag or --destTag will override this value").Default("latest").String()
	kingpin.Parse()

	if *srcArgs.Repository == "" {
		srcArgs.Repository = repoArg
	}
	if *destArgs.Repository == "" {
		destArgs.Repository = repoArg
	}

	if *srcArgs.Tag == "" {
		srcArgs.Tag = tagArg
	}
	if *destArgs.Tag == "" {
		destArgs.Tag = tagArg
	}

	if *srcArgs.Repository == "" {
		fmt.Printf("A source repository name is required either with --srcRepo or --repo\n")
		exitCode = -1
		return
	}

	if *destArgs.Repository == "" {
		fmt.Printf("A destiation repository name is required either with --destRepo or --repo\n")
		exitCode = -1
		return
	}

	srcHub, err := connectToRegistry(srcArgs, auths.Configs)
	if err != nil {
		fmt.Printf("Failed to establish a connection to the source registry. %v", err)
		exitCode = -1
		return
	}

	destHub, err := connectToRegistry(destArgs, auths.Configs)
	if err != nil {
		fmt.Printf("Failed to establish a connection to the destination registry. %v", err)
		exitCode = -1
		return
	}

	manifest, err := srcHub.Manifest(*srcArgs.Repository, *srcArgs.Tag)
	if err != nil {
		fmt.Printf("Failed to fetch the manifest for %s/%s:%s. %v", srcHub.URL, *srcArgs.Repository, *srcArgs.Tag, err)
		exitCode = -1
		return
	}

	for _, layer := range manifest.FSLayers {
		err := migrateLayer(srcHub, destHub, *srcArgs.Repository, *destArgs.Repository, layer)
		if err != nil {
			fmt.Printf("Failed to migrate image layer. %v", err)
			exitCode = -1
			return
		}
	}

	err = destHub.PutManifest(*destArgs.Repository, *destArgs.Tag, manifest)
	if err != nil {
		fmt.Printf("Failed to upload manifest to %s/%s:%s. %v", destHub.URL, *destArgs.Repository, *destArgs.Tag, err)
		exitCode = -1
	}

}
