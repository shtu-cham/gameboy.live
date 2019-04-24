package gb

import (
	"log"
)

/*
	General Memory Map
	  0000-3FFF   16KB ROM Bank 00     (in cartridge, fixed at bank 00)
	  4000-7FFF   16KB ROM Bank 01..NN (in cartridge, switchable bank number)
	  8000-9FFF   8KB Video RAM (VRAM) (switchable bank 0-1 in CGB Mode)
	  A000-BFFF   8KB External RAM     (in cartridge, switchable bank, if any)
	  C000-CFFF   4KB Work RAM Bank 0 (WRAM)
	  D000-DFFF   4KB Work RAM Bank 1 (WRAM)  (switchable bank 1-7 in CGB Mode)
	  E000-FDFF   Same as C000-DDFF (ECHO)    (typically not used)
	  FE00-FE9F   Sprite Attribute Table (OAM)
	  FEA0-FEFF   Not Usable
	  FF00-FF7F   I/O Ports
	  FF80-FFFE   High RAM (HRAM)
	  FFFF        Interrupt Enable Register
*/
type Memory struct {
	MainMemory [0x10000]byte
}

func (core *Core) initMemory() {
	log.Println("[Core] Start to initialize memory...")

	log.Println("[Memory] Load first 32KByte of rom data into memory")
	//Load first 32KB of ROM into 0000-7FFF
	for i := 0x0000; i < core.Cartridge.Props.ROMLength && i < 0x8000; i++ {
		core.Memory.MainMemory[i] = core.Cartridge.MBC.ReadRom(uint16(i))
	}

	//Specify other register mapped in main memory according to http://bgb.bircd.org/pandocs.htm#powerupsequence
	core.Memory.MainMemory[0xFF05] = 0x00
	core.Memory.MainMemory[0xFF06] = 0x00
	core.Memory.MainMemory[0xFF07] = 0x00
	core.Memory.MainMemory[0xFF0F] = 0xE1
	core.Memory.MainMemory[0xFF10] = 0x80
	core.Memory.MainMemory[0xFF11] = 0xBF
	core.Memory.MainMemory[0xFF12] = 0xF3
	core.Memory.MainMemory[0xFF14] = 0xBF
	core.Memory.MainMemory[0xFF16] = 0x3F
	core.Memory.MainMemory[0xFF17] = 0x00
	core.Memory.MainMemory[0xFF19] = 0xBF
	core.Memory.MainMemory[0xFF1A] = 0x7F
	core.Memory.MainMemory[0xFF1B] = 0xFF
	core.Memory.MainMemory[0xFF1C] = 0x9F
	core.Memory.MainMemory[0xFF1E] = 0xBF
	core.Memory.MainMemory[0xFF20] = 0xFF
	core.Memory.MainMemory[0xFF21] = 0x00
	core.Memory.MainMemory[0xFF22] = 0x00
	core.Memory.MainMemory[0xFF23] = 0xBF
	core.Memory.MainMemory[0xFF24] = 0x77
	core.Memory.MainMemory[0xFF25] = 0xF3
	core.Memory.MainMemory[0xFF26] = 0xF1
	core.Memory.MainMemory[0xFF40] = 0x91
	core.Memory.MainMemory[0xFF42] = 0x00
	core.Memory.MainMemory[0xFF43] = 0x00
	core.Memory.MainMemory[0xFF45] = 0x00
	core.Memory.MainMemory[0xFF47] = 0xFC
	core.Memory.MainMemory[0xFF48] = 0xFF
	core.Memory.MainMemory[0xFF49] = 0xFF
	core.Memory.MainMemory[0xFF4A] = 0x00
	core.Memory.MainMemory[0xFF4B] = 0x00
	core.Memory.MainMemory[0xFFFF] = 0x00

}

func (core *Core) ReadMemory(address uint16) byte {
	if (address >= 0x4000) && (address <= 0x7FFF) {
		// are we reading from the rom memory bank?
		return core.Cartridge.MBC.ReadRomBank(address)
	} else if (address >= 0xA000) && (address <= 0xBFFF) {
		// are we reading from ram memory bank?
		return core.Cartridge.MBC.ReadRomBank(address)
	}
	return core.Memory.MainMemory[address]
}

func (core *Core) WriteMemory(address uint16, data byte) {
	if address < 0x8000 {
		log.Println("todo banking")
		//core.HandleBanking(address,data) ;
	} else if (address >= 0xE000) && (address < 0xFE00) {
		// writing to ECHO ram also writes in RAM
		core.Memory.MainMemory[address] = data
		core.WriteMemory(address-0x2000, data)
	} else if (address >= 0xFEA0) && (address < 0xFEFF) {
		// this area is restricted
	} else if 0xFF04 == address {
		//This register is incremented at rate of 16384Hz (~16779Hz on SGB).
		//In CGB Double Speed Mode it is incremented twice as fast, ie. at 32768Hz.
		//Writing any value to this register resets it to 00h.
		core.Memory.MainMemory[0xFF04] = 0
	} else if address == 0xFF44 {
		//The LY indicates the vertical line to which the present data is
		//transferred to the LCD Driver. The LY can take on any value between 0 through 153.
		//The values between 144 and 153 indicate the V-Blank period. Writing will reset the counter.
		core.Memory.MainMemory[0xFF44] = 0
	} else if address == 0xFF46 {
		//FF46 - DMA - DMA Transfer and Start Address (W)
		//Writing to this register launches a DMA transfer from ROM or RAM to
		//OAM memory (sprite attribute table).
		core.DoDMA(data)
	} else if address == 0xFF07 {
		//FF07 - TAC - Timer Control (R/W)
		//  Bit 2    - Timer Stop  (0=Stop, 1=Start)
		//  Bits 1-0 - Input Clock Select
		//             00:   4096 Hz    (~4194 Hz SGB)
		//             01: 262144 Hz  (~268400 Hz SGB)
		//             10:  65536 Hz   (~67110 Hz SGB)
		//             11:  16384 Hz   (~16780 Hz SGB)
		currentFreq := core.GetClockFreq()
		core.Memory.MainMemory[0xFF07] = data
		newFreq := core.GetClockFreq()
		if currentFreq != newFreq {
			core.SetClockFreq()
		}

	} else {
		core.Memory.MainMemory[address] = data
	}
	//log.Printf("Write to %X,data:%X\n", address, data)
}

/*
	Perform DMA transfer.
	The written value specifies the transfer source address divided by 100h, ie. source & destination are:
		Source:      XX00-XX9F   ;XX in range from 00-F1h
		Destination: FE00-FE9F
*/
func (core *Core) DoDMA(data byte) {
	// source address is data * 100
	address := uint16(data) << 8
	for i := 0; i < 0xA0; i++ {
		core.WriteMemory(0xFE00+uint16(i), core.ReadMemory(address+uint16(i)))
	}
}

/*
	Push a word into stack and update SP
*/
func (core *Core) StackPush(val uint16) {
	hi := val >> 8
	lo := val & 0xFF
	core.CPU.Registers.SP--
	core.WriteMemory(core.CPU.Registers.SP, byte(hi))
	core.CPU.Registers.SP--
	core.WriteMemory(core.CPU.Registers.SP, byte(lo))

	if core.Debug {
		log.Printf("[Debug] Stack Push: %X, SP:%X", val, core.CPU.Registers.SP)
	}
}

/*
	Pop a word into stack and update SP
*/
func (core *Core) StackPop() uint16 {
	lo := core.ReadMemory(core.CPU.Registers.SP)
	hi := core.ReadMemory(core.CPU.Registers.SP + 1)
	core.CPU.Registers.SP += 2
	return uint16(lo) + (uint16(hi) << 8)
}