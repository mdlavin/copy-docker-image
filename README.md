# move-docker-image - Copy docker images without a full docker installation
[![Build Status](https://travis-ci.org/mdlavin/copy-docker-image.svg?branch=master)](https://travis-ci.org/mdlavin/copy-docker-image)

## Overview

When doing automated deployments, especially when using AWS ECR in multiple accounts, you might want to copy images from one registry to another without the need for a full docker installation. At LifeOmic we wanted to orchestrate the copying of images while executing inside a container without exposing a full Docker socker just for image manipulation.

To copy an image between two anonymous repositories, you can use a command line like:

```
$ copy-docker-image --srcRepo http://registry1/ --destRepo http://registry2 --repo project
```

To specify an image tag, just add a --tag argument like:

```
$ copy-docker-image --srcRepo http://registry1/ --destRepo http://registry2 --repo project --tag v1
```

## Integration with AWS ECR

Because copy to AWS ECR was common a special URL format was added to automatically look up the right HTTPS URL and authorization token. Assuming a AWS CLI profile has been created for your account you can use a command like:

```
$ copy-docker-image --srcRepo http://registry1/ --destRepo ecr:<account-id> --repo project
```
 
## Installation

Pre-built binaries for tagged releases are available on the [releases page](https://github.com/mdlavin/copy-docker-image/releases).
