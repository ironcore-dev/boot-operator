---
# https://vitepress.dev/reference/default-theme-home-page
layout: home

hero:
  name: "Boot Operator"
  text: "Kubernetes operator for bare metal boot provisioning"
  tagline: "Run iPXE, HTTPBoot, image proxy, and ignition endpoints as Kubernetes-native infrastructure"
  image:
    src: https://raw.githubusercontent.com/ironcore-dev/ironcore/refs/heads/main/docs/assets/logo_borderless.svg
    alt: IronCore
  actions:
    - theme: brand
      text: Getting started
      link: /quickstart
    - theme: alt
      text: API Reference
      link: /api-reference/api

features:
  - title: Declarative Boot Configuration
    details: Define per-machine boot behavior using Kubernetes resources and let controllers reconcile the full boot flow.
  - title: Secure Image Delivery
    details: Enforce OCI registry allow-lists at reconciliation and runtime while proxying images to nodes.
  - title: Native Kubernetes Integration
    details: Fully Kubernetes-native API for managing boot configuration, lifecycle, and infrastructure endpoints.
  - title: Built-in Boot Services
    details: Provides iPXE, HTTPBoot, ignition, and image proxy endpoints out of the box.
---