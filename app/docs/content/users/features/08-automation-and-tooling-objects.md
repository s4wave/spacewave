---
title: Automation and Publishing
section: features
order: 8
summary: Automate work inside your spaces and publish content from them to the public web.
draft: true
---

## What You Can Do

Two features turn your space from a private workspace into something that can *act* and *reach the outside world*:

- **Forge Dashboards** run automated workflows inside a space. Think scheduled tasks, background jobs, and repeatable pipelines that live next to the data they operate on.
- **CDN Spaces** publish content from your space to a public URL. Write a docs site or a blog privately; publish it to the web without leaving Spacewave.

Both live in the [object browser](/docs/users/spaces/object-browser). Create either one with the **+** button.

## Automating Work: Forge Dashboards

Use a Forge Dashboard when you want the space to *do* something on a schedule or in response to changes. Examples:

- Re-index your notes every night.
- Convert dropped files into a specific format.
- Poll an external feed and drop new items into the space as entries.
- Run a batch job across a large Drive.

### What the dashboard shows you

A Forge Dashboard gives you a single control panel for the workflows in a space:

- **Linked Entities** lists every task, job, worker, cluster, and run currently connected to the dashboard, with live state.
- **Process Bindings** shows which automated processes are allowed to run. Toggle a binding on to approve a process; toggle it off to revoke.

The view updates in real time as workflows progress, so you don't have to refresh to see what's happening.

### Safety

Automation is opt-in. Nothing runs until you enable a binding, and you can revoke at any time. Forge processes have the same space-scoped sandboxing as other plugins, so an automation inside one space can't reach into another.

## Publishing to the Web: CDN Space

Use a CDN Space when you've written something inside Spacewave that you want the public to be able to read — a docs site, a blog, a project page — without running your own web server.

Attach a CDN Space object to the content you want to publish (a [documentation site](/docs/users/features/documentation-sites) or a [blog](/docs/users/features/blogs) are the common targets). Spacewave's content delivery network serves the published copy at a public URL. Edits in your private space propagate to the public page.

The private side stays encrypted and under your control. The CDN only hosts the portion you explicitly publish.

## Next Steps

- [Documentation Sites](/docs/users/features/documentation-sites)
- [Blogs](/docs/users/features/blogs)
- [Virtual Machines in a Space](/docs/users/features/compute-and-network-objects)
