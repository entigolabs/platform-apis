---
sidebar_position: 1
---

# Push artifacts to AWS ECR repository

## Prerequisites

Either an AWS SSO user role or IAM user with attached policy of
```
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "ECRCreateRepository",
            "Effect": "Allow",
            "Action": "ecr:CreateRepository",
            "Resource": "*"
        },
        {
            "Sid": "ECRRepositoryAccess",
            "Effect": "Allow",
            "Action": [
                "ecr:BatchGetImage",
                "ecr:BatchCheckLayerAvailability",
                "ecr:CompleteLayerUpload",
                "ecr:GetDownloadUrlForLayer",
                "ecr:InitiateLayerUpload",
                "ecr:PutImage",
                "ecr:UploadLayerPart",
                "ecr:ListImages",
                "ecr:DescribeImages",
                "ecr:DescribeRepositories"
            ],
            "Resource": "arn:aws:ecr:<region>:<account_id>:repository/*"
        },
        {
            "Sid": "ECRAuthToken",
            "Effect": "Allow",
            "Action": "ecr:GetAuthorizationToken",
            "Resource": "*"
        }
    ]
}
```
This policy gives push/pull and create repository rights for all ECR repositories in specified account and region, should be modified as needed.

## Set up user credentials

```
export AWS_REGION=<region> # e.g eu-north-1
export AWS_ACCESS_KEY_ID="AKI....."
export AWS_SECRET_ACCESS_KEY="UO/gR....."
```
In case of SSO user session, add
```
export AWS_SESSION_TOKEN="IQoJb3....."
```

## Login to ECR 

```
aws ecr get-login-password | docker login --username AWS --password-stdin <account_id>.dkr.ecr.<region>.amazonaws.com
```

## 1. Tag and push a docker image

Create ECR repository
```
aws ecr create-repository --repository-name testimage
```
Tag docker image and push it to ECR
```
docker tag some_local_image:tag <account_id>.dkr.ecr.<region>.amazonaws.com/testimage:tag
docker push <account_id>.dkr.ecr.<region>.amazonaws.com/testimage:tag
```

## 2. Create and push an OCI Helm chart

```
aws ecr create-repository --repository-name testchart
helm create testchart
helm package testchart
helm push testchart-0.1.0.tgz <account_id>.dkr.ecr.<region>.amazonaws.com
```
Install Helm image from repository

```
helm install testchart oci://<account_id>.dkr.ecr.<region>.amazonaws.com/testchart --version 0.1.0

```
