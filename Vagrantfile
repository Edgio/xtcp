boxes = [
  {
    name: "focal-xtcp",
    os: "ubuntu/focal64",
    primary: true,
    autostart: true,
  }
]

Vagrant.configure("2") do |config|
  boxes.each do |box|
    config.vm.define box[:name], primary: !!box[:primary], autostart: !!box[:autostart] do |boxcfg|
      boxcfg.vm.box = box[:os]
    end
  end

  config.vm.synced_folder ".", "/home/vagrant/xtcp-opensource" 

  config.vm.provider "virtualbox" do |vb|
    vb.memory = 3072
    vb.cpus = 4
  end

  config.vm.provision "shell", privileged: true, inline: <<-SHELL
      cd xtcp-opensource && sudo ./bundle/scripts/install_pkgs.sh 
  SHELL

end
