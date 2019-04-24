package gb

import (
	"github.com/HFO4/gbc-in-cloud/util"
	"log"
)

type CPU struct {
	Registers Registers
	Flags     Flags
}

/*
	Registers
	  16bit Hi   Lo   Name/Function
	  AF    A    -    Accumulator & Flags
	  BC    B    C    BC
	  DE    D    E    DE
	  HL    H    L    HL
	  SP    -    -    Stack Pointer
	  PC    -    -    Program Counter/Pointer
*/
type Registers struct {
	A  byte
	B  byte
	C  byte
	D  byte
	E  byte
	F  byte
	HL uint16
	PC uint16
	SP uint16
}

/*
	The Flag Register (lower 8bit of AF register)
	  Bit  Name  Set Clr  Expl.
	  7    zf    Z   NZ   Zero Flag
	  6    n     -   -    Add/Sub-Flag (BCD)
	  5    h     -   -    Half Carry Flag (BCD)
	  4    cy    C   NC   Carry Flag
	  3-0  -     -   -    Not used (always zero)
	Conatins the result from the recent instruction which has affected flags.
*/
type Flags struct {
	Zero      bool
	Sub       bool
	HalfCarry bool
	Carry     bool
	//	IME - Interrupt Master Enable Flag (Write Only)
	//  	0 - Disable all Interrupts
	//  	1 - Enable all Interrupts that are enabled in IE Register (FFFF)
	InterruptMaster bool

	PendingInterruptDisabled bool
	PendingInterruptEnabled  bool
}

func (core *Core) initCPU() {
	log.Println("[Core] Initialize CPU flags and registers")

	//Initialize flags with default value.
	core.CPU.Flags.Zero = true
	core.CPU.Flags.Sub = false
	core.CPU.Flags.HalfCarry = true
	core.CPU.Flags.Carry = true
	core.CPU.Flags.InterruptMaster = false

	/*
		Initialize register after BIOS
		AF=$01B0
		BC=$0013
		DE=$00D8
		HL=$014D
		Stack Pointer=$FFFE
	*/
	core.CPU.Registers.A = 0x01
	core.CPU.Registers.B = 0x00
	core.CPU.Registers.C = 0x13
	core.CPU.Registers.D = 0x00
	core.CPU.Registers.E = 0xD8
	core.CPU.Registers.F = 0xB0
	core.CPU.Registers.HL = 0x014D
	core.CPU.Registers.PC = 0x0100
	core.CPU.Registers.SP = 0xFFFE

}

/*
	Execute the next  OPCode and return used CPU clock
*/
func (core *Core) ExecuteNextOPCode() int {
	opcode := core.ReadMemory(core.CPU.Registers.PC)
	core.CPU.Registers.PC++
	return core.ExecuteOPCode(opcode)
}

/*
	Execute given OPCode and return used CPU clock
*/
func (core *Core) ExecuteOPCode(code byte) int {
	if OPCodeFunctionMap[code].Clock != 0 {
		if core.Debug {
			af := core.CPU.getAF()
			bc := core.CPU.getBC()
			de := core.CPU.getDE()
			hl := core.CPU.Registers.HL
			sp := core.CPU.Registers.SP
			pc := core.CPU.Registers.PC - 1
			lcdc := core.Memory.MainMemory[0xFF40]
			IF := core.Memory.MainMemory[0xFF0F]
			IE := core.Memory.MainMemory[0xFFFF]
			log.Printf("[Debug] \n\033[34m[OP:%s]\nAF:%04X  BC:%04X  DE:%04X  HL:%04X  SP:%04X\nPC:%04X  LCDC:%02X  IF:%02X    IE:%02X    IME:%t\033[0m", OPCodeFunctionMap[code].OP, af, bc, de, hl, sp, pc, lcdc, IF, IE, core.CPU.Flags.InterruptMaster)
		}
		extCycles := OPCodeFunctionMap[code].Func(core)

		// we are trying to disable interupts, however interupts get disabled after the next instruction
		// 0xF3 is the opcode for disabling interupt
		if core.CPU.Flags.PendingInterruptDisabled {
			if core.ReadMemory(core.CPU.Registers.PC-1) != 0xF3 {
				core.CPU.Flags.PendingInterruptDisabled = false
				core.CPU.Flags.InterruptMaster = false
			}
		}

		if core.CPU.Flags.PendingInterruptEnabled {
			if core.ReadMemory(core.CPU.Registers.PC-1) != 0xFB {
				core.CPU.Flags.PendingInterruptEnabled = false
				core.CPU.Flags.InterruptMaster = true
			}
		}

		return OPCodeFunctionMap[code].Clock + extCycles
	} else {
		log.Fatalf("Unable to resolve OPCode:%X   PC:%X\n", code, core.CPU.Registers.PC-1)
		return 0
	}
}

/*
	Get 16bit parameter after opcode
*/
func (core *Core) getParameter16() uint16 {
	b1 := uint16(core.ReadMemory(core.CPU.Registers.PC))
	b2 := uint16(core.ReadMemory(core.CPU.Registers.PC + 1))
	core.CPU.Registers.PC += 2
	return b2<<8 | b1
}

/*
	Get 8bit parameter after opcode
*/
func (core *Core) getParameter8() byte {
	b := core.ReadMemory(core.CPU.Registers.PC)
	core.CPU.Registers.PC += 1
	return b
}

/*
	Get value of AF register
*/
func (cpu *CPU) getAF() uint16 {
	return uint16(cpu.Registers.A)<<8 | uint16(cpu.Registers.F)
}

/*
	Set value of AF register
*/
func (cpu *CPU) setAF(val uint16) {
	cpu.Registers.A = byte(val >> 8)
	cpu.Registers.F = byte(val & 0xFF)
}

/*
	Set value of BC register
*/
func (cpu *CPU) setBC(val uint16) {
	cpu.Registers.B = byte(val >> 8)
	cpu.Registers.C = byte(val & 0xFF)
}

/*
	Get value of BC register
*/
func (cpu *CPU) getBC() uint16 {
	return uint16(cpu.Registers.B)<<8 | uint16(cpu.Registers.C)
}

/*
	Get value of DE register
*/
func (cpu *CPU) getDE() uint16 {
	return uint16(cpu.Registers.D)<<8 | uint16(cpu.Registers.E)
}

/*
	Update Low 8bit of AF register
*/
func (cpu *CPU) updateAFLow() {
	newAF := cpu.Registers.F
	if cpu.Flags.Zero {
		newAF = util.SetBit(newAF, 7)
	} else {
		newAF = util.ClearBit(newAF, 7)
	}

	if cpu.Flags.Sub {
		newAF = util.SetBit(newAF, 6)
	} else {
		newAF = util.ClearBit(newAF, 6)
	}

	if cpu.Flags.HalfCarry {
		newAF = util.SetBit(newAF, 5)
	} else {
		newAF = util.ClearBit(newAF, 5)
	}

	if cpu.Flags.Carry {
		newAF = util.SetBit(newAF, 4)
	} else {
		newAF = util.ClearBit(newAF, 4)
	}

	cpu.Registers.F = newAF

}

/*
	Compare two values and set flags
*/

func (cpu *CPU) Compare(val1 byte, val2 byte) {
	cpu.Flags.Zero = (val1 == val2)
	cpu.Flags.Carry = (val1 > val2)
	cpu.Flags.HalfCarry = ((val1 & 0x0f) > (val2 & 0x0f))
	cpu.Flags.Sub = true

	cpu.updateAFLow()

}