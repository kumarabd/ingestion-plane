# Building the Documentation Site

## Issue Fixed

The navigation links were broken because the URLs in `_data/toc.yml` didn't match the permalinks in the markdown files.

**Fixed:** All URLs now have consistent format: `/docs/section/page/`

## How to Build

### Option 1: Local Jekyll Server

```bash
cd docs

# Install dependencies (first time only)
bundle install

# Serve locally
bundle exec jekyll serve

# Site will be available at:
# http://localhost:4000
```

### Option 2: Build Static Site

```bash
cd docs

# Build the site
bundle exec jekyll build

# Output will be in _site/ directory
# Deploy _site/ contents to your web server
```

### Option 3: GitHub Pages

If you're using GitHub Pages, simply push to your repository:

```bash
git add .
git commit -m "Update documentation with fixed navigation"
git push origin main
```

GitHub Pages will automatically build and deploy the site.

## Verify Links Work

After building, verify these URLs work:

### Learn Section
- http://localhost:4000/docs/learn/overview/
- http://localhost:4000/docs/learn/architecture/
- http://localhost:4000/docs/learn/overview/problem-strategy/
- http://localhost:4000/docs/learn/overview/use-cases/

### Implement Section
- http://localhost:4000/docs/implement/getting-started/
- http://localhost:4000/docs/implement/user-guide/
- http://localhost:4000/docs/implement/troubleshooting/

### Reference Section
- http://localhost:4000/docs/reference/component-specs/
- http://localhost:4000/docs/reference/gateway-service/
- http://localhost:4000/docs/reference/component-services/
- http://localhost:4000/docs/reference/data-contracts/
- http://localhost:4000/docs/reference/api-reference/

### Other
- http://localhost:4000/about/
- http://localhost:4000/ (homepage)

## Troubleshooting

### "Bundle not found"

```bash
gem install bundler
bundle install
```

### "Jekyll not found"

```bash
gem install jekyll
```

### Changes not showing

```bash
# Stop the server (Ctrl+C)
# Clear the cache
rm -rf _site .jekyll-cache

# Restart
bundle exec jekyll serve
```

### Links still broken

1. Check that all markdown files have correct frontmatter:
   ```yaml
   ---
   layout: page
   title: Your Title
   permalink: /docs/section/page/
   ---
   ```

2. Check that `_data/toc.yml` URLs match permalinks exactly

3. Rebuild the site from scratch:
   ```bash
   rm -rf _site .jekyll-cache
   bundle exec jekyll serve
   ```

## What Was Changed

### Files Updated

1. **`_data/toc.yml`** ✅
   - Added leading slashes to all URLs: `/docs/...`
   - Added trailing slashes to all URLs: `.../page/`
   - Now matches permalink format exactly

2. **New Documentation Files** ✅
   - `_docs/learn/architecture.md` - Full system architecture
   - `_docs/reference/gateway-service.md` - Gateway details
   - `_docs/reference/component-services.md` - All services
   - All have correct permalink format

3. **Updated Files** ✅
   - `_docs/learn/overview.md` - Updated content
   - `README.md` - Updated project README
   - `DOCUMENTATION_INDEX.md` - Navigation guide

### Permalink Format

All pages now use consistent permalink format:

```yaml
permalink: /docs/section/page/
```

With:
- Leading slash: `/`
- Full path: `/docs/section/page`
- Trailing slash: `/`

## Next Steps

1. Build the site locally to verify
2. Check all navigation links work
3. Commit and push changes
4. Deploy to production (GitHub Pages or your server)

---

**Last Updated:** January 2024  
**Status:** ✅ Navigation Fixed

