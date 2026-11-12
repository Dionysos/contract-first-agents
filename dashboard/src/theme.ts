// Thème Bootstrap (data-bs-theme) : celui du système, ou forcé par ?theme=light|dark.
export function applyTheme() {
  const forced = new URLSearchParams(location.search).get('theme');
  const media = window.matchMedia('(prefers-color-scheme: dark)');
  const set = () => {
    const theme = forced === 'light' || forced === 'dark' ? forced : media.matches ? 'dark' : 'light';
    document.documentElement.dataset.bsTheme = theme;
  };
  set();
  media.addEventListener('change', set);
}
