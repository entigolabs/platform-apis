---
sidebar_position: 1
---

# Introduction

Entigo Platform is an Internal Developer Platform (IDP) for software development teams working with containerized applications. It provides a unified API built on the Kubernetes API model, enabling you to manage both applications and infrastructure without juggling multiple tools and ecosystems.

This documentation serves two primary audiences: **software engineers** who build, deploy, and run applications and infrastructure, and **platform operators** who customize and maintain the platform to meet organizational needs.

## What Entigo Platform Provides

Entigo Platform delivers a cohesive developer experience by integrating common open-source technologies through templating and meaningful defaults. Rather than exposing raw infrastructure primitives, the platform combines them into higher-level building blocks that align with how software engineers think about their systems.

**For software engineers**, the platform currently enables you to:
- Create isolated security segments for applications and infrastructure services
- Launch infrastructure services like databases and in-memory stores commonly used by applications
- Deploy applications using standard Kubernetes API primitives

**For platform operators**, the platform enables you to:
- Define cross-team defaults and guardrails to meet compliance requirements
- Customize platform behavior while maintaining a consistent developer experience

## How It Works

Entigo Platform builds on [Infralib](https://www.entigo.com/infralib) and extends Kubernetes APIs, integrating technologies including Crossplane for infrastructure management, ArgoCD for GitOps workflows, and AWS cloud services.

## Documentation Structure

This documentation is organized into two sections:

- [**User Documentation**](user/) - Covers key features, APIs, and examples for common software delivery workflows
- [**Operator Documentation**](operator/) - Explores technical implementation details, customization patterns, and platform configuration

---

*New to the platform? Start with [Quick Start Guide](#) to deploy your first application, or explore [Core Concepts](#) to understand the platform's architecture.*