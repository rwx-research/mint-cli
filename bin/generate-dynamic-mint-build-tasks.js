const YAML = require("yaml");

const tasks = [];
const osArr = ["linux", "darwin", "windows"];
const archArr = [
  {
    arch: "amd64",
    newArch: "x86_64",
  },
  { arch: "arm64", newArch: "aarch64" },
];

osArr.forEach((os) => {
  archArr.forEach((arch) => {
    tasks.push({
      key: `build-mint-${os}-${arch.arch}`,
      use: "setup-nix",
      run: `
        GOOS=${os} \
        GOARCH=${arch.arch} \
        CGO_ENABLED=0 \
        LDFLAGS="-w -s -X github.com/rwx-research/mint-cli/cmd/mint/config.Version=${process.env.FULL_VERSION}" \
          nix develop --command mage
      `,
    });
    if (os === "darwin") {
      tasks.push({
        key: `notarize-${arch.arch}-binary`,
        use: [
          "setup-codesigning",
          "install-zip",
          `build-mint-${os}-${arch.arch}`,
        ],
        run: `
          echo "$RWX_APPLE_DEVELOPER_ID_APPLICATION_CERT" > rwx-developer-id-application-cert.pem
          # first we sign the binary. This happens locally.
          ./rcodesign sign --pem-source rwx-developer-id-application-cert.pem --code-signature-flags runtime "./mint"
          # notarizing requires certain container formats, that's why we zip
          zip -r mint.zip "./mint"
          echo "$RWX_APPLE_APP_STORE_CONNECT_API_KEY" > rwx-apple-app-store-connect-api-key.json
          ./rcodesign notary-submit --wait --api-key-path rwx-apple-app-store-connect-api-key.json mint.zip
        `,
        env: {
          CODESIGN_VERSION: "0.22.0",
          RWX_APPLE_DEVELOPER_ID_APPLICATION_CERT:
            "${{ secrets.RWX_APPLE_DEVELOPER_ID_APPLICATION_CERT }}",
          RWX_APPLE_APP_STORE_CONNECT_API_KEY:
            "${{ secrets.RWX_APPLE_APP_STORE_CONNECT_API_KEY }}",
        },
      });
    }
    tasks.push({
      key: `upload-${os}-${arch.arch}-to-release`,
      use:
        os === "darwin"
          ? `notarize-${arch.arch}-binary`
          : `build-mint-${os}-${arch.arch}`,
      run: `
        ${os === "windows" ? 'extension=".exe"' : 'extension=""'}
        github_asset_name=$(echo "mint-${os}-${
        arch.newArch
      }$extension" | tr '[:upper:]' '[:lower:]')
        mv "mint$extension" "$github_asset_name"
        gh release upload ${
          process.env.FULL_VERSION
        } "$github_asset_name" --clobber
        `,
      env: {
        GH_TOKEN: "${{ secrets.MINT_CLI_REPO_GH_TOKEN }}",
      },
    });
  });
});

console.log(YAML.stringify(tasks));
