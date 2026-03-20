import { withMermaid } from "vitepress-plugin-mermaid";
import { fileURLToPath, URL } from "node:url";

export default withMermaid({
  title: "Boot Operator",
  description: "Kubernetes operator to automate bare metal network boot infrastructure",
  base: "/boot-operator/",
  head: [
    [
      "link",
      {
        rel: "icon",
        href: "https://raw.githubusercontent.com/ironcore-dev/ironcore/refs/heads/main/docs/assets/logo_borderless.svg",
      },
    ],
  ],
  vite: {
    resolve: {
      alias: [
        {
          // Override default theme footer with our funding notice.
          find: /^.*\/VPFooter\.vue$/,
          replacement: fileURLToPath(
            new URL("./theme/components/VPFooter.vue", import.meta.url),
          ),
        },
      ],
    },
  },
  themeConfig: {
    nav: [
      { text: "Home", link: "/" },
      { text: "Documentation", link: "/architecture" },
      { text: "Quickstart", link: "/quickstart" },
      { text: "IronCore Documentation", link: "https://ironcore-dev.github.io" },
    ],

    editLink: {
      pattern: "https://github.com/ironcore-dev/boot-operator/blob/main/docs/:path",
      text: "Edit this page on GitHub",
    },

    logo: {
      src: "https://raw.githubusercontent.com/ironcore-dev/ironcore/refs/heads/main/docs/assets/logo_borderless.svg",
      width: 24,
      height: 24,
    },

    search: {
      provider: "local",
    },

    sidebar: [
      {
        items: [
          { text: "Quickstart", link: "/quickstart" },
          {
            text: "Installation",
            collapsed: true,
            items: [
              { text: "Kustomize", link: "/installation/kustomize" },
              { text: "Helm", link: "/installation/helm" },
            ],
          },
          { text: "Architecture", link: "/architecture" },
          { text: "API Reference", link: "/api-reference/api" },
        ],
      },
      {
        text: "Usage",
        collapsed: false,
        items: [{ text: "bootctl", link: "/usage/bootctl" }],
      },
      {
        text: "Development",
        collapsed: false,
        items: [
          { text: "Documentation", link: "/development/dev_docs" },
          { text: "Create UKI", link: "/development/create_uki" },
        ],
      },
    ],

    socialLinks: [
      { icon: "github", link: "https://github.com/ironcore-dev/boot-operator" },
    ],
  },
});

