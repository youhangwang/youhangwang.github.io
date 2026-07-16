# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# OpenWolf

@.wolf/OPENWOLF.md

This project uses OpenWolf for context management. Read and follow .wolf/OPENWOLF.md every session. Check .wolf/cerebrum.md before generating code. Check .wolf/anatomy.md before reading files.

## What this repo is

A personal Jekyll blog ("Johan Blog") hosted on GitHub Pages at `youhangwang.github.io`. It uses the **`jekyll-text-theme`** gem (v2.2.6), but ships a local, customized copy of the theme's `_layouts/`, `_includes/`, and `_sass/` — these local files **override** the gem. Edit theme files in those directories; do not assume changes to the gem propagate.

Content is primarily **Chinese** (`lang: zh`, timezone `Asia/Shanghai`). Post topics are cloud-native / Kubernetes / storage infra (CSI, Ramen/regional-DR, NooBaa, kubevirt, ODF, CRI resource management, Go internals).

## Commands

### Prerequisites — install Ruby + Jekyll (macOS)

**Do not use the system Ruby** (macOS ships an old Ruby 2.6 — `bundle exec jekyll build` fails with `Could not find 'bundler' (2.3.19)` on this machine). Per the official [Jekyll macOS guide](https://jekyllrb.com/docs/installation/macos/), install a modern Ruby via a version manager. `chruby` is the recommended simplest option:

```bash
# 1. Homebrew (if not installed)
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# 2. chruby + ruby-install, then a Jekyll-supported Ruby (3.4.x)
brew install chruby ruby-install
ruby-install ruby 3.4.1

# 3. auto-activate in zsh, then relaunch Terminal
echo "source $(brew --prefix)/opt/chruby/share/chruby/chruby.sh" >> ~/.zshrc
echo "source $(brew --prefix)/opt/chruby/share/chruby/auto.sh" >> ~/.zshrc
echo "chruby ruby-3.4.1" >> ~/.zshrc

ruby -v   # should report 3.4.1+, NOT system 2.6
```

The toolchain is Ruby 3.4.1 + Jekyll 4.2.2 + `jekyll-text-theme` 2.2.6. Jekyll is pulled in via the `Gemfile` (no need for a bare `gem install jekyll`). Two Ruby 3.4 compat fixes are already baked into the repo — **do not remove them**:

- The `Gemfile` lists `csv`, `base64`, `bigdecimal`, `logger`, `ostruct`, `singleton`, `benchmark` — Ruby 3.4 unbundled these from the stdlib and Jekyll 4.2.x still `require`s them; under `bundle exec` they must be in the Gemfile.
- `_plugins/ruby3_compat.rb` restores `Object#tainted?`/`taint`/`untaint` as no-ops — Ruby 3.2+ removed them but `liquid-4.0.3` (pinned by jekyll 4.2.x) still calls `tainted?`.

**China network (this machine):** `rubygems.org` is unreachable here. The `gem sources` mirror does **not** apply to bundler — `bundle install` reads the source from the `Gemfile`. Configure the bundler mirror once (stored in `.bundle/config`, keeps the Gemfile clean):

```bash
gem sources --remove https://rubygems.org/
gem sources -a https://gems.ruby-china.com/
bundle config set mirror.https://rubygems.org https://gems.ruby-china.com
```

### Running the site

Always `source chruby && chruby ruby-3.4.1` first, then run Jekyll through `bundle exec` to pin the gem versions in `Gemfile.lock`:

```bash
source "$(brew --prefix)/opt/chruby/share/chruby/chruby.sh" && chruby ruby-3.4.1
bundle install                    # install Ruby deps (first time / after Gemfile change)
bundle exec jekyll serve          # dev server at http://127.0.0.1:4000 with live reload
bundle exec jekyll serve --drafts # include _draft/ posts in the dev server
bundle exec jekyll build          # production build into _site/
bundle exec jekyll clean          # remove _site/, .jekyll-cache/, Sass cache
bundle exec jekyll doctor         # report deprecations / config issues
```

Never run two `bundle install` concurrently on this repo — they deadlock on the bundler lock. Useful flags (see [Build Command Options](https://jekyllrb.com/docs/configuration/options/)): `--config _alt.yml` (override/merge config), `-V` (verbose), `--incremental` (faster partial rebuilds), `--livereload` (explicit).

`_config.yml` is **not** hot-reloaded by `jekyll serve` — restart the server after editing it. There are no tests, linters, or build scripts beyond Jekyll. Deployment is automatic via GitHub Pages on push to the default branch — do not commit the generated `_site/` (it is ignored).

## Architecture

- **`_config.yml`** — central config. Note `permalink: date` (post URLs are `/:year/:month/:day/:title.html`), `paginate: 10`, `excerpt_separator: <!--more-->`, and feature flags for `mathjax`, `mermaid`, and `chart` (all enabled). Config changes are **not** hot-reloaded — restart `jekyll serve` after editing.
- **`_posts/`** — published posts, filename format `YYYY-MM-DD-title.md` (Jekyll enforces this). Front matter typically just `title` + `tags`; the `defaults` block in `_config.yml` auto-applies `layout: article`, TOC aside, license, sharing, and pageview to every post.
- **`_draft/`** — unpublished drafts (not rendered unless `--drafts`). Contains long-form infra analysis posts plus embedded `.drawio.html` diagrams.
- **`src/`** — Go source files referenced/embedded inside posts (e.g. `src/json-patch/`, `src/strategic-merge-patch/`). These are **not** compiled or tested by the blog; they exist as readable source for articles. Treat edits here as content, not buildable code.
- **`_layouts/` + `_includes/`** — the customized theme layer. `base.html` is the root HTML shell; `article.html` / `articles.html` render posts; `landing.html` / `page.html` render static pages. Includes are organized by concern (`article/`, `aside/`, `head/`, `scripts/`, `sidebar/`, `snippets/`).
- **`_sass/`** — theme stylesheets. `custom.scss` is the intended entry point for site-specific style overrides; prefer editing it over touching core theme partials under `skins/`, `components/`, `layout/`.
- **`_data/`** — site data: `authors.yml`, `licenses.yml`, `locale.yml` (i18n strings), `navigation.yml` (nav menus), `variables.yml`.
- **`openspec/`** — OpenSpec spec/change tracking (unrelated to the Jekyll build; do not let it affect `_site/`).

## Conventions for new posts

- Place in `_posts/` with `YYYY-MM-DD-title.md`. Use `<!--more-->` to mark the excerpt break shown on listing pages.
- Tags are space-separated in front matter (e.g. `tags: golang rpc`).
- Math (MathJax), Mermaid, and Chart.js blocks are available in post bodies without extra setup — wrap Mermaid in a `<pre class="mermaid">` block per the theme's convention.
- License defaults to `CC-BY-NC-4.0` from `_config.yml`; override per-post in front matter if needed.
