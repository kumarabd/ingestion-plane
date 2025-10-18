# Rebuild Instructions - Documentation Updated

## What Was Changed

### 1. Site Configuration (`_config.yml`)
- ✅ Title: "Log Analyzer" → "Ingestion Plane"
- ✅ Description: Updated to reflect dual Loki, template mining, semantic search
- ✅ Repository: Updated to "ingestion-plane"

### 2. Navigation (`_data/toc.yml`)
- ✅ All URLs now match permalink format: `/docs/section/page/`
- ✅ Added "Architecture" to Learn section
- ✅ Added "Gateway Service" to Reference section
- ✅ Added "Component Services" to Reference section

### 3. About Page (`pages/about.md`)
- ✅ Updated from "Log Analyzer" to "Ingestion Plane"
- ✅ Updated content to reflect current architecture
- ✅ Added microservices information
- ✅ Updated technology stack

### 4. New Documentation
- ✅ `_docs/learn/architecture.md` - Complete system architecture
- ✅ `_docs/reference/gateway-service.md` - Detailed Gateway docs
- ✅ `_docs/reference/component-services.md` - All services documented

### 5. Fixed Build Issues
- ✅ Escaped Liquid syntax in LogQL examples
- ✅ Created placeholder `assets/css/style.scss`
- ✅ All pages build successfully

## How to View Updated Site

### Stop Any Running Server

```bash
# Find and kill any running Jekyll processes
pkill -f jekyll
```

### Clear Cache and Rebuild

```bash
cd docs

# Clear all caches
rm -rf _site .jekyll-cache

# Rebuild site
bundle exec jekyll build

# Start server
bundle exec jekyll serve --host 0.0.0.0
```

### Access Site

Open in browser: http://localhost:4000

### Force Browser Refresh

After server starts, **force refresh** in browser:
- **Mac**: Cmd + Shift + R
- **Windows/Linux**: Ctrl + Shift + R

This clears browser cache and loads fresh content.

## Verify Changes

Check these areas:

### Header/Title
- Should say "Ingestion Plane" (not "Log Analyzer")

### Left Sidebar Navigation
Should show:
- **Learn**
  - Overview
  - Architecture ← NEW
  - Problem & Strategy
  - Use Cases
- **Implement**
  - Getting Started
  - User Guide
  - Troubleshooting
- **Reference**
  - Component Specifications
  - Gateway Service ← NEW
  - Component Services ← NEW
  - Data Contracts & APIs
  - API Reference

### Test These Pages
- http://localhost:4000/docs/learn/architecture/
- http://localhost:4000/docs/reference/gateway-service/
- http://localhost:4000/docs/reference/component-services/
- http://localhost:4000/about/

## Still Seeing Old Content?

### 1. Clear Browser Cache Completely
**Chrome/Edge:**
- Settings → Privacy → Clear browsing data
- Select "Cached images and files"
- Click "Clear data"

**Firefox:**
- Settings → Privacy & Security → Cookies and Site Data
- Click "Clear Data"

**Safari:**
- Develop → Empty Caches (or Cmd+Option+E)

### 2. Use Incognito/Private Mode
Open http://localhost:4000 in an incognito/private window

### 3. Check Jekyll is Serving Fresh Build
```bash
# Stop server
pkill -f jekyll

# Clean everything
cd docs
rm -rf _site .jekyll-cache .sass-cache

# Rebuild
bundle exec jekyll build

# Verify files exist
ls -la _site/docs/learn/architecture/
ls -la _site/docs/reference/gateway-service/

# Start server
bundle exec jekyll serve --host 0.0.0.0 --watch
```

### 4. Check Server Output
When you access a page, you should see in the terminal:
```
127.0.0.1 - - [date] "GET /docs/learn/architecture/ HTTP/1.1" 200
```

If you see 404 errors, the pages aren't being generated.

## Summary of All URL Changes

| Section | Page | URL |
|---------|------|-----|
| Learn | Overview | `/docs/learn/overview/` |
| Learn | **Architecture** | `/docs/learn/architecture/` ← NEW |
| Learn | Problem & Strategy | `/docs/learn/overview/problem-strategy/` |
| Learn | Use Cases | `/docs/learn/overview/use-cases/` |
| Reference | Component Specs | `/docs/reference/component-specs/` |
| Reference | **Gateway Service** | `/docs/reference/gateway-service/` ← NEW |
| Reference | **Component Services** | `/docs/reference/component-services/` ← NEW |
| Reference | Data Contracts | `/docs/reference/data-contracts/` |
| Reference | API Reference | `/docs/reference/api-reference/` |
| Other | About | `/about/` (content updated) |

## Files You Can Delete

These helper files can be removed after verifying the site works:
- `docs/BUILD_SITE.md`
- `docs/QUICK_START.md`
- `docs/REBUILD_INSTRUCTIONS.md` (this file)
- `docs/DOCUMENTATION_INDEX.md` (optional - keep as reference)

---

**Status:** ✅ All updates complete  
**Last Updated:** October 2024  
**Build Status:** Success (0.3 seconds)

