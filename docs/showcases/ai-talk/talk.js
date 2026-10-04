(() => {
  const stage = document.getElementById('stage');
  const slides = [...stage.querySelectorAll('.slide')];
  const hooks = {};
  let cur = 0;
  const steps = slides.map(() => 0);

  // ---------- storage (best effort) ----------
  const store = {
    get(k) { try { return localStorage.getItem(k); } catch { return null; } },
    set(k, v) { try { localStorage.setItem(k, v); } catch { /* private mode */ } },
  };
  const theme = store.get('kept-talk-theme');
  if (theme) document.documentElement.dataset.theme = theme;

  // ---------- scaling ----------
  function fit() {
    const s = Math.min(window.innerWidth / 1600, window.innerHeight / 900) * 0.96;
    stage.style.transform = `scale(${s}) translate(-50%, -50%)`;
  }
  window.addEventListener('resize', fit);
  fit();

  // ---------- steps ----------
  function maxStep(i) {
    const h = hooks[slides[i].id];
    if (h && h.steps != null) return h.steps;
    let m = 0;
    slides[i].querySelectorAll('.f').forEach(el => { m = Math.max(m, +el.dataset.f || 0); });
    return m;
  }

  function applyStep(i) {
    const s = slides[i];
    const n = steps[i];
    s.querySelectorAll('.f').forEach(el => el.classList.toggle('on', (+el.dataset.f || 0) <= n));
    const h = hooks[s.id];
    if (h && h.onStep) h.onStep(n);
  }

  function show(i, stepTo) {
    i = Math.max(0, Math.min(slides.length - 1, i));
    const prev = cur;
    cur = i;
    if (stepTo === 'end') steps[i] = maxStep(i);
    else if (typeof stepTo === 'number') steps[i] = stepTo;
    slides.forEach((s, k) => {
      s.classList.toggle('is-current', k === i);
      s.classList.toggle('is-past', k < i);
    });
    applyStep(i);
    if (prev !== i) {
      const ph = hooks[slides[prev].id];
      if (ph && ph.onLeave) ph.onLeave();
    }
    document.getElementById('counter').textContent = `${i + 1} / ${slides.length}`;
    document.getElementById('progress').style.width = `${((i + 1) / slides.length) * 100}%`;
    history.replaceState(null, '', `#${i + 1}`);
    document.querySelectorAll('#index-list button').forEach((b, k) => b.classList.toggle('cur', k === i));
  }

  function next() {
    if (steps[cur] < maxStep(cur)) { steps[cur]++; applyStep(cur); }
    else if (cur < slides.length - 1) show(cur + 1, 0);
  }
  function prev() {
    if (steps[cur] > 0) { steps[cur]--; applyStep(cur); }
    else if (cur > 0) show(cur - 1, 'end');
  }

  // ---------- index ----------
  const index = document.getElementById('index');
  const list = document.getElementById('index-list');
  slides.forEach((s, k) => {
    const li = document.createElement('li');
    const b = document.createElement('button');
    b.innerHTML = `<span>${String(k + 1).padStart(2, '0')}${s.querySelector('.draft') ? ' · drafted' : ''}</span>`;
    b.append(s.dataset.title || '');
    b.addEventListener('click', () => { toggleIndex(false); show(k, 0); });
    li.append(b);
    list.append(li);
  });
  function toggleIndex(force) { index.classList.toggle('open', force); }
  index.addEventListener('click', e => { if (e.target === index) toggleIndex(false); });

  function toggleTheme() {
    const r = document.documentElement;
    r.dataset.theme = r.dataset.theme === 'light' ? 'dark' : 'light';
    store.set('kept-talk-theme', r.dataset.theme);
  }

  // ---------- input ----------
  document.addEventListener('keydown', e => {
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    const k = e.key;
    if (index.classList.contains('open') && k === 'Escape') return toggleIndex(false);
    if (['ArrowRight', 'PageDown', ' ', 'Enter'].includes(k)) { e.preventDefault(); next(); }
    else if (['ArrowLeft', 'PageUp', 'Backspace'].includes(k)) { e.preventDefault(); prev(); }
    else if (k === 'ArrowDown') { e.preventDefault(); show(cur + 1, 0); }
    else if (k === 'ArrowUp') { e.preventDefault(); show(cur - 1, 0); }
    else if (k === 'Home') show(0, 0);
    else if (k === 'End') show(slides.length - 1, 0);
    else if (k === 'o' || k === 'Escape') toggleIndex();
    else if (k === 'f') { document.fullscreenElement ? document.exitFullscreen() : document.documentElement.requestFullscreen?.(); }
    else if (k === 't') toggleTheme();
    else if (k === 'd') document.body.classList.toggle('no-draft');
  });
  stage.addEventListener('click', e => {
    if (e.target.closest('button, a, .term, .tree')) return;
    const r = stage.getBoundingClientRect();
    (e.clientX - r.left < r.width * 0.25) ? prev() : next();
  });
  let tx = null;
  stage.addEventListener('touchstart', e => { tx = e.touches[0].clientX; }, { passive: true });
  stage.addEventListener('touchend', e => {
    if (tx == null) return;
    const dx = e.changedTouches[0].clientX - tx;
    if (Math.abs(dx) > 40) (dx < 0 ? next : prev)();
    tx = null;
  });
  document.getElementById('btn-next').onclick = next;
  document.getElementById('btn-prev').onclick = prev;
  document.getElementById('btn-index').onclick = () => toggleIndex();
  document.getElementById('btn-theme').onclick = toggleTheme;

  // =====================================================================
  // workflow walkthrough
  // =====================================================================
  (() => {
    const wf = document.getElementById('wf');
    const svg = wf.querySelector('svg');
    const dots = document.getElementById('wf-dots');
    const color = v => getComputedStyle(document.documentElement).getPropertyValue(v).trim();

    const STEPS = [
      { c: '--green', nodes: [], edges: [] },
      { c: '--green',
        nodes: ['n-ws', 'n-ratify', 'n-retire', 'n-ledger', 'n-acts'],
        edges: ['e-ws-ratify', 'e-ratify-acts', 'e-ws-retire', 'e-retire-acts'] },
      { c: '--teal',
        nodes: ['n-ws', 'n-tick', 'n-ledger', 'n-truths', 'n-obs', 'n-disp', 'n-state'],
        edges: ['e-ws-tick', 'e-tick-truths', 'e-tick-obs', 'e-tick-disp'] },
      { c: '--amber',
        nodes: ['n-ledger', 'n-acts', 'n-truths', 'n-drift', 'n-results'],
        edges: ['e-acts-drift', 'e-truths-drift', 'e-drift-results'] },
      { c: '--purple',
        nodes: ['n-repair', 'n-ledger', 'n-disp', 'n-receipts', 'n-acts', 'n-packet', 'n-attempt', 'n-state'],
        edges: ['e-disp-repair', 'e-receipts-repair', 'e-repair-acts', 'e-repair-attempt', 'e-ledger-attempt', 'e-state-attempt'],
        turns: true },
      { c: '--blue',
        nodes: ['n-review', 'n-state', 'n-ledger', 'n-receipts', 'n-assess'],
        edges: ['e-state-review', 'e-ledger-review', 'e-review-assess'] },
      { c: '--green',
        nodes: ['n-land', 'n-state', 'n-ws', 'n-ledger', 'n-acts'],
        edges: ['e-state-land', 'e-land-ws', 'e-land-acts'] },
    ];

    const caps = [...wf.querySelectorAll('.wf-cap')];
    const chips = [...wf.querySelectorAll('#wf-steps button[data-s]')];
    const turns = [...wf.querySelectorAll('.turn')];
    let raf = null, turnTimer = null, active = [], playTimer = null;

    function particles() {
      cancelAnimationFrame(raf);
      dots.innerHTML = '';
      const runners = active.map(id => {
        const p = document.getElementById(id);
        const len = p.getTotalLength();
        const bi = p.classList.contains('bi');
        const n = Math.max(1, Math.round(len / 140));
        return [...Array(n)].map((_, k) => {
          const c = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
          c.setAttribute('r', 4); c.setAttribute('class', 'dot');
          dots.append(c);
          return { c, p, len, bi, off: k / n, rev: bi && k % 2 === 1 };
        });
      }).flat();
      const t0 = performance.now();
      const tick = t => {
        const s = ((t - t0) / 1000);
        for (const r of runners) {
          let u = (s * 110 / r.len + r.off) % 1;
          if (r.rev) u = 1 - u;
          const pt = r.p.getPointAtLength(u * r.len);
          r.c.setAttribute('cx', pt.x); r.c.setAttribute('cy', pt.y);
          r.c.style.opacity = Math.min(1, Math.sin(Math.PI * ((s * 110 / r.len + r.off) % 1)) * 2.2);
        }
        raf = requestAnimationFrame(tick);
      };
      if (runners.length) raf = requestAnimationFrame(tick);
    }

    function runTurns(on) {
      clearInterval(turnTimer);
      turns.forEach(t => t.classList.remove('lit'));
      if (!on) return;
      let k = 0;
      turnTimer = setInterval(() => {
        if (k >= turns.length) { turns.forEach(t => t.classList.remove('lit')); k = 0; return; }
        turns[k++].classList.add('lit');
      }, 520);
    }

    function set(n) {
      const st = STEPS[n];
      wf.style.setProperty('--hotc', color(st.c));
      wf.classList.toggle('focus', n > 0);
      svg.querySelectorAll('.node, .edge').forEach(el => el.classList.remove('hot'));
      st.nodes.forEach(id => document.getElementById(id).classList.add('hot'));
      st.edges.forEach(id => document.getElementById(id).classList.add('hot'));
      // a hot ledger keeps its quiet rows half-visible, so it still reads as one column
      const ledgerHot = n > 0 && st.nodes.includes('n-ledger');
      ['n-acts', 'n-truths', 'n-obs', 'n-disp', 'n-receipts'].forEach(id => {
        const el = document.getElementById(id);
        el.style.opacity = ledgerHot && !el.classList.contains('hot') ? .5 : '';
      });
      active = st.edges;
      particles();
      runTurns(!!st.turns);
      caps.forEach(c => c.classList.toggle('cur', +c.dataset.s === n));
      chips.forEach(c => c.classList.toggle('cur', +c.dataset.s === n));
    }

    function stopPlay() {
      clearInterval(playTimer); playTimer = null;
      document.getElementById('wf-play').textContent = '▶ play';
    }

    chips.forEach(c => c.addEventListener('click', () => {
      stopPlay();
      steps[slides.indexOf(document.getElementById('workflow'))] = +c.dataset.s;
      set(+c.dataset.s);
    }));
    document.getElementById('wf-play').addEventListener('click', e => {
      if (playTimer) return stopPlay();
      e.currentTarget.textContent = '❚❚ pause';
      const idx = slides.indexOf(document.getElementById('workflow'));
      const advance = () => {
        steps[idx] = steps[idx] >= 6 ? 1 : steps[idx] + 1;
        set(steps[idx]);
      };
      advance();
      playTimer = setInterval(advance, 11000);
    });

    hooks.workflow = {
      steps: 6,
      onStep: set,
      onLeave() { stopPlay(); cancelAnimationFrame(raf); clearInterval(turnTimer); dots.innerHTML = ''; },
    };
  })();

  // =====================================================================
  // gather → observe → evaluate → drift
  // =====================================================================
  (() => {
    const boxes = [...document.querySelectorAll('#pipe .stagebox')];
    let world = 'clean', standing = 'ratified';
    const $ = id => document.getElementById(id);

    const WORLDS = {
      clean:  { tree: 'tree 9f31…', sub: 'internal/gather/command.go changed, so the promise is re-checked', gather: 'exit 0', obs: '{ "ok": true }', verdict: 'kept' },
      broken: { tree: 'tree c07a…', sub: 'command.go now has fmt.Printf("%d", name) — it compiles, but vet flags it', gather: 'exit 1 — vet found a problem', obs: '{ "ok": false }', verdict: 'not-kept' },
      notool: { tree: 'tree c07a…', sub: 'same change, but Go isn\'t installed on this machine', gather: 'could not run → no observation', obs: '—', verdict: 'unknown' },
    };
    const CLS = { kept: 'verdict-kept', 'not-kept': 'verdict-not', unknown: 'verdict-unknown' };

    function render(flash) {
      const w = WORLDS[world];
      $('g-world').textContent = w.tree;
      $('g-world-sub').textContent = w.sub;
      $('g-gather-sub').textContent = w.gather;
      $('g-obs').textContent = w.obs;
      $('g-verdict').innerHTML = `<span class="${CLS[w.verdict]}">${w.verdict}</span>`;
      const ratified = standing === 'ratified';
      $('g-standing').textContent = ratified ? 'ratified' : 'none';
      $('g-standing').className = ratified ? 'verdict-kept' : 'muted';
      $('g-truth').textContent = w.verdict;
      $('g-truth').className = CLS[w.verdict];
      const drift = ratified && w.verdict !== 'kept';
      $('g-drift').innerHTML = drift
        ? `<span class="${CLS[w.verdict]}">drifting</span>`
        : '<span class="verdict-kept">not drifting</span>';
      $('g-drift-sub').textContent = !drift
        ? (ratified ? 'promised, and holding' : 'nobody ratified it, so nothing is owed')
        : (w.verdict === 'not-kept' ? 'kept repair can take it from here' : 'needs a person to find out why it could not measure');
      if (flash) rerun();
    }

    document.querySelectorAll('[data-world]').forEach(b => b.addEventListener('click', () => {
      world = b.dataset.world;
      document.querySelectorAll('[data-world]').forEach(x => x.classList.toggle('cur', x === b));
      render(true);
    }));
    document.querySelectorAll('[data-standing]').forEach(b => b.addEventListener('click', () => {
      standing = b.dataset.standing;
      document.querySelectorAll('[data-standing]').forEach(x => x.classList.toggle('cur', x === b));
      render(true);
    }));

    // a changed world re-runs the pipeline: every result clears, then each stage
    // answers in turn, left to right
    let timers = [];
    function rerun() {
      timers.forEach(clearTimeout); timers = [];
      const lit = boxes.filter(b => b.classList.contains('lit'));
      lit.forEach(b => b.classList.remove('shown'));
      lit.forEach((b, k) => timers.push(setTimeout(() => b.classList.add('shown'), 250 + k * 380)));
    }

    hooks.gather = {
      onStep(n) {
        timers.forEach(clearTimeout); timers = [];
        boxes.forEach(b => {
          const on = +b.dataset.i <= Math.max(n, 0);
          b.classList.toggle('lit', on);
          b.classList.toggle('shown', on);
        });
      },
    };
    render(false);
  })();

  // =====================================================================
  // repair: swimlanes and the thread card fill in as the steps run
  // =====================================================================
  (() => {
    const slide = document.getElementById('thread');
    if (!slide) return;
    const els = [...slide.querySelectorAll('[data-at], [data-until]')];
    const cols = [...slide.querySelectorAll('[data-col]')];
    const wrap = slide.querySelector('.lanes-wrap');
    hooks.thread = {
      steps: 13,
      onStep(n) {
        els.forEach(el => {
          const at = el.dataset.at != null ? +el.dataset.at : -Infinity;
          const until = el.dataset.until != null ? +el.dataset.until : Infinity;
          el.style.display = n >= at && n < until ? '' : 'none';
          el.classList.toggle('just', at === n && !el.classList.contains('anat-attempt'));
        });
        cols.forEach(c => {
          c.classList.toggle('cur', +c.dataset.col === n);
          c.classList.toggle('done', +c.dataset.col < n);
        });
        // keep the current step in view; the timeline is wider than the slide
        const head = slide.querySelector(`.col-h[data-col="${n}"]`);
        if (wrap && head) {
          const label = 112;
          const target = head.offsetLeft - label - (wrap.clientWidth - label - head.offsetWidth) / 2;
          wrap.scrollLeft = Math.max(0, target);
        } else if (wrap) wrap.scrollLeft = 0;
      },
    };
  })();

  // ---------- start ----------
  // #12 opens slide 12; #12.3 opens it at step 3
  const [hs, hstep] = location.hash.slice(1).split('.').map(n => parseInt(n, 10));
  show(Number.isFinite(hs) ? hs - 1 : 0, Number.isFinite(hstep) ? Math.min(hstep, maxStep(hs - 1)) : 0);
})();
