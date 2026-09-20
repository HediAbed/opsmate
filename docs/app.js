function wireTabs(tablist) {
  const tabs = Array.from(tablist.querySelectorAll('[role="tab"]'));
  if (tabs.length === 0) {
    return;
  }

  const show = (tab) => {
    tabs.forEach((other) => {
      const panel = document.getElementById(other.getAttribute("aria-controls"));
      const selected = other === tab;
      other.setAttribute("aria-selected", String(selected));
      other.classList.toggle("ptab--on", selected);
      other.tabIndex = selected ? 0 : -1;
      if (panel) {
        panel.hidden = !selected;
        panel.classList.toggle("pane--on", selected);
      }
    });
  };

  tablist.addEventListener("click", (event) => {
    const tab = event.target.closest('[role="tab"]');
    if (tab) {
      show(tab);
    }
  });

  tablist.addEventListener("keydown", (event) => {
    const current = tabs.indexOf(document.activeElement);
    if (current < 0) {
      return;
    }
    const step = event.key === "ArrowRight" ? 1 : event.key === "ArrowLeft" ? -1 : 0;
    if (step === 0) {
      return;
    }
    event.preventDefault();
    const next = tabs[(current + step + tabs.length) % tabs.length];
    next.focus();
    show(next);
  });
}

document.querySelectorAll('[role="tablist"], .os').forEach(wireTabs);
