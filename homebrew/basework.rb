# Homebrew Formula for basework
# 此文件由 goreleaser 自动更新

class Basework < Formula
  desc "AI Agent 框架和独立终端产品"
  homepage "https://github.com/wly2lcl/basework"
  version "1.3.0"
  
  if OS.mac?
    if Hardware::CPU.arm?
      url "https://github.com/wly2lcl/basework/releases/download/v#{version}/basework_#{version}_darwin_arm64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256"
    else
      url "https://github.com/wly2lcl/basework/releases/download/v#{version}/basework_#{version}_darwin_amd64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256"
    end
  elsif OS.linux?
    if Hardware::CPU.arm?
      url "https://github.com/wly2lcl/basework/releases/download/v#{version}/basework_#{version}_linux_arm64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256"
    else
      url "https://github.com/wly2lcl/basework/releases/download/v#{version}/basework_#{version}_linux_amd64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256"
    end
  end

  def install
    bin.install "basework"
  end

  test do
    system "#{bin}/basework --version"
  end
end
